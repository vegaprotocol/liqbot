package bot

import (
	"errors"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
)

// Bot is the generic bot interface.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/bot_mock.go -package mocks code.vegaprotocol.io/liqbot/bot Bot
type Bot interface {
	Start() error
	Stop()
	GetTraderDetails() string
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// New returns a new Bot instance.
func New(botConf config.BotConfig, seedConf *config.SeedConfig, pe PricingEngine, wc normal.WalletClient) (Bot, error) {
	switch botConf.Strategy {
	case config.BotStrategyNormal:
		return normal.New(botConf, seedConf, pe, wc), nil
	default:
		return nil, errors.New("unrecognised bot strategy")
	}
}
