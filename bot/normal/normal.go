package normal

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/vega/wallet/wallets"
)

// bot represents one Normal liquidity bot.
type bot struct {
	walletClient WalletClient
	marketService
	accountService

	config config.BotConfig
	log    *log.Entry

	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	pausePosMgmt    chan struct{}
	pausePriceSteer chan struct{}

	walletPubKey      string
	marketID          string
	decimalPlaces     uint64
	settlementAssetID string
	vegaAssetID       string
	botPaused         bool
	mu                sync.Mutex
}

// TODO: there could be a service that would be in charge of managing the account, balance of the account, creating markets
// and simplifying the process of placing orders.
// The bot should only have to worry decision making about what orders to place and when, given the current market state.
// The service should be responsible for the following:
// - creating markets (if necessary)
// - placing orders, as produced by the bot
// - managing the account balance

// New returns a new instance of bot.
func New(
	botConf config.BotConfig,
	vegaAssetID string,
	wc WalletClient,
	accountService accountService,
	marketService marketService,
) *bot {
	return &bot{
		config:            botConf,
		settlementAssetID: botConf.SettlementAssetID,
		vegaAssetID:       vegaAssetID,
		log: log.WithFields(log.Fields{
			"bot": botConf.Name,
		}),

		stopPosMgmt:     make(chan bool),
		stopPriceSteer:  make(chan bool),
		pausePosMgmt:    make(chan struct{}),
		pausePriceSteer: make(chan struct{}),
		accountService:  accountService,
		marketService:   marketService,
		walletClient:    wc,
	}
}

// Start starts the liquidity bot goroutine(s).
func (b *bot) Start() error {
	setupLogger(true) // TODO: pretty from config?

	ctx := context.Background()

	walletPubKey, err := b.setupWallet(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup wallet: %w", err)
	}

	b.walletPubKey = walletPubKey

	pauseCh := b.pauseChannel()

	b.accountService.Init(walletPubKey, pauseCh)

	if err = b.marketService.Init(walletPubKey, pauseCh); err != nil {
		return fmt.Errorf("failed to init market service: %w", err)
	}

	market, err := b.SetupMarket(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup market: %w", err)
	}

	b.marketID = market.Id
	b.decimalPlaces = market.DecimalPlaces

	if err = b.marketService.Start(market.Id); err != nil {
		return fmt.Errorf("failed to start market service: %w", err)
	}

	b.log.WithFields(log.Fields{
		"id":                b.marketID,
		"base/ticker":       b.config.InstrumentBase,
		"quote":             b.config.InstrumentQuote,
		"settlementAssetID": b.settlementAssetID,
	}).Info("Market info")

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		defer cancel()
		b.runPositionManagement(ctx)
	}()

	go func() {
		defer cancel()
		b.runPriceSteering(ctx)
	}()

	return nil
}

func setupLogger(pretty bool) {
	log.SetReportCaller(true)
	log.SetFormatter(&log.JSONFormatter{
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			function := strings.ReplaceAll(f.Function, "code.vegaprotocol.io/", "")
			idx := strings.Index(function, ".")
			function = fmt.Sprintf("%s/%s/%s():%d", function[:idx], filename, function[idx+1:], f.Line)
			return function, ""
		},
		PrettyPrint: pretty,
		DataKey:     "_vals",
		FieldMap: log.FieldMap{
			log.FieldKeyMsg: "_msg",
		},
	})
}

func (b *bot) pauseChannel() chan types.PauseSignal {
	in := make(chan types.PauseSignal)
	go func() {
		for {
			select {
			case p := <-in:
				b.Pause(p)
			}
		}
	}()
	return in
}

// Pause pauses the liquidity bot goroutine(s).
func (b *bot) Pause(p types.PauseSignal) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.Pause && !b.botPaused {
		b.log.WithFields(log.Fields{"From": p.From}).Info("Pausing bot")
		b.botPaused = true
	} else if !p.Pause && b.botPaused {
		b.log.WithFields(log.Fields{"From": p.From}).Info("Resuming bot")
		b.botPaused = false
	} else {
		return
	}

	select {
	case b.pausePosMgmt <- struct{}{}:
	default:
	}
	select {
	case b.pausePriceSteer <- struct{}{}:
	default:
	}
}

// Stop stops the liquidity bot goroutine(s).
func (b *bot) Stop() {
	select {
	case <-b.stopPosMgmt:
	default:
		close(b.stopPosMgmt)
	}

	select {
	case <-b.stopPriceSteer:
	default:
		close(b.stopPriceSteer)
	}
}

// GetTraderDetails returns information relating to the trader.
func (b *bot) GetTraderDetails() string {
	jsn, _ := json.MarshalIndent(map[string]string{
		"name":                  b.config.Name,
		"pubKey":                b.walletPubKey,
		"settlementVegaAssetID": b.settlementAssetID,
	}, "", "  ")
	return string(jsn)
}

func (b *bot) setupWallet(ctx context.Context) (string, error) {
	walletPassphrase := "123"

	if err := b.walletClient.LoginWallet(ctx, b.config.Name, walletPassphrase); err != nil {
		if strings.Contains(err.Error(), wallets.ErrWalletDoesNotExists.Error()) {
			if err = b.walletClient.CreateWallet(ctx, b.config.Name, walletPassphrase); err != nil {
				return "", fmt.Errorf("failed to create wallet: %w", err)
			}
			b.log.Info("Created and logged into wallet")
		} else {
			return "", fmt.Errorf("failed to log into wallet: %w", err)
		}
	}

	b.log.Info("Logged into wallet")

	publicKeys, err := b.walletClient.ListPublicKeys(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list public keys: %w", err)
	}

	var walletPubKey string

	if len(publicKeys) == 0 {
		key, err := b.walletClient.GenerateKeyPair(ctx, walletPassphrase, []types.Meta{})
		if err != nil {
			return "", fmt.Errorf("failed to generate keypair: %w", err)
		}
		walletPubKey = key.Pub
		b.log.WithFields(log.Fields{"pubKey": walletPubKey}).Debug("Created keypair")
	} else {
		walletPubKey = publicKeys[0]
		b.log.WithFields(log.Fields{"pubKey": walletPubKey}).Debug("Using existing keypair")
	}

	b.log = b.log.WithFields(log.Fields{"pubkey": walletPubKey})

	return walletPubKey, nil
}
