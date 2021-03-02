package normal

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/node"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

// Node is a Vega gRPC node
//go:generate go run github.com/golang/mock/mockgen -destination mocks/node_mock.go -package mocks code.vegaprotocol.io/liqbot/bot Node
type Node interface {
	GetAddress() (url.URL, error)

	// Trading

	// Trading Data
	GetVegaTime() (time.Time, error)
	MarketByID(req *api.MarketByIDRequest) (response *api.MarketByIDResponse, err error)
	PartyAccounts(req *api.PartyAccountsRequest) (response *api.PartyAccountsResponse, err error)
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// LiqBot represents one liquidity bot.
type LiqBot struct {
	config          config.BotConfig
	active          bool
	log             *log.Entry
	pricingEngine   PricingEngine
	settlementAsset string
	stop            chan bool
	market          *proto.Market
	node            Node

	walletServer     wallet.WalletHandler
	walletPassphrase string
	walletPubKeyRaw  []byte // "XYZ" ...
	walletPubKeyHex  string // "58595a" ...
	walletToken      string

	mu sync.Mutex
}

// New returns a new instance of LiqBot.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) *LiqBot {
	lb := LiqBot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine: pe,
		walletServer:  ws,
	}

	return &lb
}

// Start starts the liquidity bot goroutine(s).
func (lb *LiqBot) Start() error {
	lb.mu.Lock()
	err := lb.setupWallet()
	if err != nil {
		lb.mu.Unlock()
		return errors.Wrap(err, "failed to start bot")
	}

	lb.node, err = node.NewGRPCNode(
		url.URL{Host: lb.config.Location},
		time.Duration(lb.config.ConnectTimeout)*time.Millisecond,
		time.Duration(lb.config.CallTimeout)*time.Millisecond,
	)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Vega gRPC node")
	}
	lb.log.WithFields(log.Fields{
		"address": lb.config.Location,
	}).Debug("Connected to Vega gRPC node")

	marketResponse, err := lb.node.MarketByID(&api.MarketByIDRequest{MarketId: lb.config.MarketID})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get market %s", lb.config.MarketID))
	}
	lb.market = marketResponse.Market
	future := lb.market.TradableInstrument.Instrument.GetFuture()
	if future == nil {
		return errors.New("market is not a Futures market")
	}
	lb.settlementAsset = future.SettlementAsset
	lb.log.WithFields(log.Fields{
		"marketID":        lb.config.MarketID,
		"settlementAsset": lb.settlementAsset,
	}).Debug("Fetched market info")

	lb.active = true
	lb.stop = make(chan bool)
	lb.mu.Unlock()

	go lb.run()

	return nil
}

// Stop stops the liquidity bot goroutine(s).
func (lb *LiqBot) Stop() {
	lb.mu.Lock()
	active := lb.active
	lb.mu.Unlock()

	if !active {
		return
	}

	// Do not send data to lb.stop while lb.mu is held. It would cause deadlock.
	// This goroutine would be trying to send on a channel while the lock is
	// held, and another goroutine (lb.run()) needs to aquire the lock before
	// the channel is read from.
	lb.stop <- true
	close(lb.stop)
}

func (lb *LiqBot) run() {
	for {
		select {
		case <-lb.stop:
			lb.log.Debug("Stopping bot")
			lb.active = false
			return

		default:
			blockTime, err := lb.node.GetVegaTime()
			if err != nil {
				lb.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to get block time")
			} else {
				lb.log.WithFields(log.Fields{"blockTime": blockTime}).Debug("Got block time")
			}

			accounts, err := lb.node.PartyAccounts(&api.PartyAccountsRequest{
				PartyId: lb.walletPubKeyHex,
				Asset:   lb.settlementAsset,
			})
			if err != nil {
				lb.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to get accounts")
			} else {
				lb.log.WithFields(log.Fields{"partyID": lb.walletPubKeyHex, "accounts": len(accounts.Accounts)}).Debug("Got accounts")
				for i, acc := range accounts.Accounts {
					lb.log.WithFields(log.Fields{
						"asset":   acc.Asset,
						"balance": acc.Balance,
						"i":       i,
						"type":    acc.Type,
					}).Debug("Got account")
				}
			}

			lb.log.Debug("Sleeping...")
			err = doze(time.Duration(5)*time.Second, lb.stop)
			if err != nil {
				lb.log.Debug("Stopping bot")
				lb.active = false
				return
			}
		}
	}
}

func (lb *LiqBot) setupWallet() (err error) {
	// no need to acquire lb.mu, it's already held.
	lb.walletPassphrase = "DCBAabcd1357!#&*" + lb.config.Name

	if lb.walletToken == "" {
		lb.walletToken, err = lb.walletServer.LoginWallet(lb.config.Name, lb.walletPassphrase)
		if err != nil {
			if err == wallet.ErrWalletDoesNotExists {
				lb.walletToken, err = lb.walletServer.CreateWallet(lb.config.Name, lb.walletPassphrase)
				if err != nil {
					return errors.Wrap(err, "failed to create wallet")
				}
				lb.log.Debug("Created and logged into wallet")
			} else {
				return errors.Wrap(err, "failed to log in to wallet")
			}
		} else {
			lb.log.Debug("Logged into wallet")
		}
	}

	if lb.walletPubKeyHex == "" || lb.walletPubKeyRaw == nil {
		var keys []wallet.Keypair
		keys, err = lb.walletServer.ListPublicKeys(lb.walletToken)
		if err != nil {
			return errors.Wrap(err, "failed to list public keys")
		}
		if len(keys) == 0 {
			lb.walletPubKeyHex, err = lb.walletServer.GenerateKeypair(lb.walletToken, lb.walletPassphrase)
			if err != nil {
				return errors.Wrap(err, "failed to generate keypair")
			}
			lb.log.WithFields(log.Fields{"pubKey": lb.walletPubKeyHex}).Debug("Created keypair")
		} else {
			lb.walletPubKeyHex = keys[0].Pub
			lb.log.WithFields(log.Fields{"pubKey": lb.walletPubKeyHex}).Debug("Using existing keypair")
		}

		lb.walletPubKeyRaw, err = hexToRaw([]byte(lb.walletPubKeyHex))
		if err != nil {
			lb.walletPubKeyHex = ""
			lb.walletPubKeyRaw = nil
			return errors.Wrap(err, "failed to decode hex pubkey")
		}
	}
	return
}

func doze(d time.Duration, stop chan bool) error {
	interval := 100 * time.Millisecond
	for d > interval {
		select {
		case <-stop:
			return e.ErrInterrupted

		default:
			time.Sleep(interval)
			d -= interval
		}
	}
	time.Sleep(d)
	return nil
}

func hexToRaw(hexBytes []byte) ([]byte, error) {
	raw := make([]byte, hex.DecodedLen(len(hexBytes)))
	n, err := hex.Decode(raw, hexBytes)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode hex")
	}
	if n != len(raw) {
		return nil, fmt.Errorf("failed to decode hex: decoded %d bytes, expected to decode %d bytes", n, len(raw))
	}
	return raw, nil
}
