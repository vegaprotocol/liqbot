package bot

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/core"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// LiqBot represents one liquidity bot.
type LiqBot struct {
	config        config.BotConfig
	active        bool
	pricingEngine PricingEngine
	stop          chan bool

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
		config:        config,
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
	lb.mu.Lock()
	sublog := log.WithFields(log.Fields{
		"trader": lb.config.Name,
		"node":   lb.config.Location,
	})

	lb.mu.Unlock()

	for {
		select {
		case <-lb.stop:
			lb.active = false
			return

		default:
			sublog.Debug("Sleeping...")
			err := doze(time.Second, lb.stop)
			if err != nil {
				lb.active = false
				return
			}
		}
	}
}

func (lb *LiqBot) setupWallet() (err error) {
	// no need to acquire lb.mu, it's already held.
	sublog := log.WithFields(log.Fields{"name": lb.config.Name})

	lb.walletPassphrase = "DCBAabcd1357!#&*" + lb.config.Name

	if lb.walletToken == "" {
		lb.walletToken, err = lb.walletServer.LoginWallet(lb.config.Name, lb.walletPassphrase)
		if err != nil {
			if err == wallet.ErrWalletDoesNotExists {
				lb.walletToken, err = lb.walletServer.CreateWallet(lb.config.Name, lb.walletPassphrase)
				if err != nil {
					return errors.Wrap(err, "failed to create wallet")
				}
				sublog.Debug("Created and logged into wallet")
			} else {
				return errors.Wrap(err, "failed to log in to wallet")
			}
		} else {
			sublog.Debug("Logged into wallet")
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
			sublog.WithFields(log.Fields{"pubKey": lb.walletPubKeyHex}).Debug("Created keypair")
		} else {
			lb.walletPubKeyHex = keys[0].Pub
			sublog.WithFields(log.Fields{"pubKey": lb.walletPubKeyHex}).Debug("Using existing keypair")
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
			return core.ErrInterrupted

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
