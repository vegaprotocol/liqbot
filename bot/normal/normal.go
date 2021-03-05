package normal

import (
	"encoding/base64"
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
//go:generate go run github.com/golang/mock/mockgen -destination mocks/node_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal Node
type Node interface {
	GetAddress() (url.URL, error)

	// Trading
	SubmitTransaction(req *api.SubmitTransactionRequest) (resp *api.SubmitTransactionResponse, err error)

	// Trading Data
	GetVegaTime() (time.Time, error)
	LiquidityProvisions(req *api.LiquidityProvisionsRequest) (response *api.LiquidityProvisionsResponse, err error)
	MarketByID(req *api.MarketByIDRequest) (response *api.MarketByIDResponse, err error)
	PartyAccounts(req *api.PartyAccountsRequest) (response *api.PartyAccountsResponse, err error)
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// Bot represents one Normal liquidity bot.
type Bot struct {
	config          config.BotConfig
	active          bool
	log             *log.Entry
	pricingEngine   PricingEngine
	settlementAsset string
	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	strategy        *Strategy
	market          *proto.Market
	node            Node

	walletServer     wallet.WalletHandler
	walletPassphrase string
	walletPubKeyRaw  []byte // "XYZ" ...
	walletPubKeyHex  string // "58595a" ...
	walletToken      string

	mu sync.Mutex
}

// New returns a new instance of Bot.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) (b *Bot, err error) {
	b = &Bot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine: pe,
		walletServer:  ws,
	}

	b.strategy, err = readStrategyConfig(config.StrategyDetails)
	if err != nil {
		err = errors.Wrap(err, "failed to read strategy details")
		return
	}
	b.log.WithFields(log.Fields{
		"strategy": b.strategy.String(),
	}).Debug("read strategy config")

	return
}

// Start starts the liquidity bot goroutine(s).
func (b *Bot) Start() error {
	b.mu.Lock()
	err := b.setupWallet()
	if err != nil {
		b.mu.Unlock()
		return errors.Wrap(err, "failed to start bot")
	}

	b.node, err = node.NewGRPCNode(
		url.URL{Host: b.config.Location},
		time.Duration(b.config.ConnectTimeout)*time.Millisecond,
		time.Duration(b.config.CallTimeout)*time.Millisecond,
	)
	if err != nil {
		return errors.Wrap(err, "failed to connect to Vega gRPC node")
	}
	b.log.WithFields(log.Fields{
		"address": b.config.Location,
	}).Debug("Connected to Vega gRPC node")

	marketResponse, err := b.node.MarketByID(&api.MarketByIDRequest{MarketId: b.config.MarketID})
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get market %s", b.config.MarketID))
	}
	b.market = marketResponse.Market
	future := b.market.TradableInstrument.Instrument.GetFuture()
	if future == nil {
		return errors.New("market is not a Futures market")
	}
	b.settlementAsset = future.SettlementAsset
	b.log.WithFields(log.Fields{
		"marketID":        b.config.MarketID,
		"settlementAsset": b.settlementAsset,
	}).Debug("Fetched market info")

	b.active = true
	b.stopPosMgmt = make(chan bool)
	b.stopPriceSteer = make(chan bool)
	b.mu.Unlock()

	go b.runPositionManagement()
	go b.runPriceSteering()

	return nil
}

// Stop stops the liquidity bot goroutine(s).
func (b *Bot) Stop() {
	b.mu.Lock()
	active := b.active
	b.mu.Unlock()

	if !active {
		return
	}

	// Do not send data to b.stop* while b.mu is held. It would cause deadlock.
	// This goroutine would be trying to send on a channel while the lock is
	// held, and another goroutine (b.run()) needs to aquire the lock before
	// the channel is read from.
	b.stopPosMgmt <- true
	close(b.stopPosMgmt)
	b.stopPriceSteer <- true
	close(b.stopPriceSteer)
}

// ConvertSignedBundle converts from trading-core.wallet.SignedBundle to trading-core.proto.api.SignedBundle
func ConvertSignedBundle(sb *wallet.SignedBundle) *proto.SignedBundle {
	/*
		From: wallet.SignedBundle:

		type SignedBundle struct {
			Tx  []byte
			Sig *wallet.Signature
		}

		To: proto.api.SignedBundle:

		type SignedBundle struct {
			Tx  []byte
			Sig *proto.Signature
		}
	*/
	return &proto.SignedBundle{
		Tx: sb.Tx,
		Sig: &proto.Signature{
			Sig:     sb.Sig.Sig,
			Algo:    sb.Sig.Algo,
			Version: sb.Sig.Version,
		},
	}
}

func (b *Bot) signSubmitTx(blob []byte, typ api.SubmitTransactionRequest_Type) error {
	// Sign, using internal wallet server
	blobBase64 := base64.StdEncoding.EncodeToString(blob)
	signedBundle, err := b.walletServer.SignTx(b.walletToken, blobBase64, b.walletPubKeyHex)
	if err != nil {
		return errors.Wrap(err, "failed to sign tx")
	}

	// Submit TX
	sub := &api.SubmitTransactionRequest{
		Tx:   ConvertSignedBundle(&signedBundle),
		Type: typ,
	}
	submitResponse, err := b.node.SubmitTransaction(sub)
	if err != nil {
		return errors.Wrap(err, "failed to submit signed tx")
	}
	if !submitResponse.Success {
		return errors.New("failed to submit signed tx (success=false)")
	}
	return nil
}

func (b *Bot) submitLiquidityProvision(sub *api.PrepareLiquidityProvisionRequest) error {
	msg := "failed to submit liquidity provision"
	// Prepare tx, without talking to a Vega node
	prepared, err := node.PrepareLiquidityProvision(sub)
	if err != nil {
		return errors.Wrap(err, msg)
	}

	err = b.signSubmitTx(prepared.Blob, api.SubmitTransactionRequest_TYPE_COMMIT)
	if err != nil {
		return errors.Wrap(err, msg)
	}
	return nil
}

func (b *Bot) manageLiquidityProvision() error {
	b.mu.Lock()
	marketID := b.market.Id
	partyID := b.walletPubKeyHex
	b.mu.Unlock()

	lpReq := &api.LiquidityProvisionsRequest{
		Market: marketID,
		Party:  partyID,
	}
	lpResponse, err := b.node.LiquidityProvisions(lpReq)
	if err != nil {
		return errors.Wrap(err, "failed to get liquidity provisions")
	}
	if len(lpResponse.LiquidityProvisions) > 0 {
		// This bot already has a liquidity provision
		return nil
	}

	buys := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -20, Proportion: 50},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_BID, Offset: -10, Proportion: 50},
	}
	sells := []*proto.LiquidityOrder{
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 10, Proportion: 50},
		{Reference: proto.PeggedReference_PEGGED_REFERENCE_BEST_ASK, Offset: 20, Proportion: 50},
	}
	sub := &api.PrepareLiquidityProvisionRequest{
		Submission: &proto.LiquidityProvisionSubmission{
			Fee:              "0.01",
			MarketId:         marketID,
			CommitmentAmount: 100,
			Buys:             buys,
			Sells:            sells,
		},
	}
	err = b.submitLiquidityProvision(sub)
	if err != nil {
		return errors.Wrap(err, "failed to submit liquidity provision order")
	}
	return nil
}

func (b *Bot) runPositionManagement() {
	// b.mu.Lock()
	// marketID := b.market.Id
	// partyID := b.walletPubKeyHex
	// b.mu.Unlock()

	for {
		select {
		case <-b.stopPosMgmt:
			b.log.Debug("Stopping bot position management")
			b.mu.Lock()
			b.active = false
			b.mu.Unlock()
			return

		default:
			b.log.Debug("Position management tick")

			err := b.manageLiquidityProvision()
			if err != nil {
				b.log.WithFields(log.Fields{
					"error": err.Error(),
				}).Warning("Failed to manage liquidity provision")
			}

			// blockTime, err := b.node.GetVegaTime()
			// if err != nil {
			// 	b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to get block time")
			// } else {
			// 	b.log.WithFields(log.Fields{"blockTime": blockTime}).Debug("Got block time")
			// }

			// accounts, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
			// 	PartyId: b.walletPubKeyHex,
			// 	Asset:   b.settlementAsset,
			// })
			// if err != nil {
			// 	b.log.WithFields(log.Fields{"error": err.Error()}).Warning("Failed to get accounts")
			// } else {
			// 	b.log.WithFields(log.Fields{"partyID": b.walletPubKeyHex, "accounts": len(accounts.Accounts)}).Debug("Got accounts")
			// 	for i, acc := range accounts.Accounts {
			// 		b.log.WithFields(log.Fields{
			// 			"asset":   acc.Asset,
			// 			"balance": acc.Balance,
			// 			"i":       i,
			// 			"type":    acc.Type,
			// 		}).Debug("Got account")
			// 	}
			// }

			err = doze(time.Duration(b.strategy.PosManagementSleepMilliseconds)*time.Millisecond, b.stopPosMgmt)
			if err != nil {
				b.log.Debug("Stopping bot position management")
				b.mu.Lock()
				b.active = false
				b.mu.Unlock()
				return
			}
		}
	}
}

func (b *Bot) runPriceSteering() {
	for {
		select {
		case <-b.stopPriceSteer:
			b.log.Debug("Stopping bot market price steering")
			b.mu.Lock()
			b.active = false
			b.mu.Unlock()
			return

		default:
			b.log.Debug("Market price steering tick")

			err := doze(time.Duration(1.0/b.strategy.MarketPriceSteeringRatePerSecond)*time.Second, b.stopPriceSteer)
			if err != nil {
				b.log.Debug("Stopping bot market price steering")
				b.mu.Lock()
				b.active = false
				b.mu.Unlock()
				return
			}
		}
	}
}

func (b *Bot) setupWallet() (err error) {
	// no need to acquire b.mu, it's already held.
	b.walletPassphrase = "DCBAabcd1357!#&*" + b.config.Name

	if b.walletToken == "" {
		b.walletToken, err = b.walletServer.LoginWallet(b.config.Name, b.walletPassphrase)
		if err != nil {
			if err == wallet.ErrWalletDoesNotExists {
				b.walletToken, err = b.walletServer.CreateWallet(b.config.Name, b.walletPassphrase)
				if err != nil {
					return errors.Wrap(err, "failed to create wallet")
				}
				b.log.Debug("Created and logged into wallet")
			} else {
				return errors.Wrap(err, "failed to log in to wallet")
			}
		} else {
			b.log.Debug("Logged into wallet")
		}
	}

	if b.walletPubKeyHex == "" || b.walletPubKeyRaw == nil {
		var keys []wallet.Keypair
		keys, err = b.walletServer.ListPublicKeys(b.walletToken)
		if err != nil {
			return errors.Wrap(err, "failed to list public keys")
		}
		if len(keys) == 0 {
			b.walletPubKeyHex, err = b.walletServer.GenerateKeypair(b.walletToken, b.walletPassphrase)
			if err != nil {
				return errors.Wrap(err, "failed to generate keypair")
			}
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKeyHex}).Debug("Created keypair")
		} else {
			b.walletPubKeyHex = keys[0].Pub
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKeyHex}).Debug("Using existing keypair")
		}

		b.walletPubKeyRaw, err = hexToRaw([]byte(b.walletPubKeyHex))
		if err != nil {
			b.walletPubKeyHex = ""
			b.walletPubKeyRaw = nil
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
