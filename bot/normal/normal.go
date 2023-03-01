package normal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/shared/libs/types"
	"code.vegaprotocol.io/vega/logging"
	"code.vegaprotocol.io/vega/protos/vega"
)

// bot represents one Normal liquidity bot.
type bot struct {
	marketService
	accountService

	config config.BotConfig
	log    *logging.Logger

	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	pausePosMgmt    chan struct{}
	pausePriceSteer chan struct{}
	pauseChannel    chan types.PauseSignal

	walletPubKey    string
	marketID        string
	decimalPlaces   uint64
	settlementAsset *vega.Asset
	vegaAssetID     string
	botPaused       bool
	mu              sync.Mutex
}

// New returns a new instance of bot.
func New(
	log *logging.Logger,
	botConf config.BotConfig,
	vegaAssetID string,
	settlementAsset *vega.Asset,
	accountService accountService,
	marketService marketService,
	pauseChannel chan types.PauseSignal,
) *bot {
	return &bot{
		config:          botConf,
		settlementAsset: settlementAsset,
		vegaAssetID:     vegaAssetID,
		log:             log,
		stopPosMgmt:     make(chan bool),
		stopPriceSteer:  make(chan bool),
		pausePosMgmt:    make(chan struct{}),
		pausePriceSteer: make(chan struct{}),
		accountService:  accountService,
		marketService:   marketService,
		pauseChannel:    pauseChannel,
	}
}

// Start starts the liquidity bot goroutine(s).
func (b *bot) Start() error {
	ctx := context.Background()

	go func() {
		for p := range b.pauseChannel {
			b.Pause(p)
		}
	}()

	market, err := b.SetupMarket(ctx)
	if err != nil {
		return fmt.Errorf("failed to setup market: %w", err)
	}

	b.marketID = market.Id
	b.decimalPlaces = market.DecimalPlaces

	b.log.With(
		logging.String("id", b.marketID),
		logging.String("base/ticker", b.config.InstrumentBase),
		logging.String("quote", b.config.InstrumentQuote),
		logging.String("settlementAssetID", b.settlementAsset.Id),
	).Info("Market info")

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

func (b *bot) PauseChannel() chan types.PauseSignal {
	return b.pauseChannel
}

// Pause pauses the liquidity bot goroutine(s).
func (b *bot) Pause(p types.PauseSignal) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.Pause && !b.botPaused {
		b.log.With(logging.String("From", p.From)).Info("Pausing bot")
		b.botPaused = true
	} else if !p.Pause && b.botPaused {
		b.log.With(logging.String("From", p.From)).Info("Resuming bot")
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
		"settlementVegaAssetID": b.settlementAsset.Id,
	}, "", "  ")
	return string(jsn)
}
