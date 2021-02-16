package bot

import (
	"code.vegaprotocol.io/liqbot/config"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	tcwallet "code.vegaprotocol.io/vega/wallet"
)

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// LiqBot represents one liquidity bot.
type LiqBot struct {
	config        config.BotConfig
	pricingEngine PricingEngine
	walletServer  tcwallet.WalletHandler
}

// New returns a new instance of LiqBot.
func New(config config.BotConfig, pe PricingEngine, ws tcwallet.WalletHandler) *LiqBot {
	lb := LiqBot{
		config:        config,
		pricingEngine: pe,
		walletServer:  ws,
	}
	return &lb
}

// Start starts the liquidity bot goroutine(s).
func (lb *LiqBot) Start() {
	go lb.run()
}

// Stop stops the liquidity bot goroutine(s).
func (lb *LiqBot) Stop() {
	// TBD
}

func (lb *LiqBot) run() {
	// TBD
}
