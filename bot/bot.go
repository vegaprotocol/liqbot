package bot

import (
	"fmt"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
)

// Bot is the generic bot interface.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/bot_mock.go -package mocks code.vegaprotocol.io/liqbot/bot Bot
type Bot interface {
	Start() error
	Stop()
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// New returns a new Bot instance.
func New(config config.BotConfig, pe PricingEngine, ws wallet.WalletHandler) (b Bot, err error) {
	switch config.Strategy {
	case "normal":
		b = normal.New(config, pe, ws)
	default:
		err = fmt.Errorf("unrecognised bot strategy: %s", config.Strategy)
	}
	return
}
