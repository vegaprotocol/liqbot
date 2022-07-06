package bot

import (
	"errors"
	"fmt"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types"
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
func New(config config.BotConfig, ethereumAddress string, pe PricingEngine, wc types.WalletClient) (b Bot, err error) {
	switch config.Strategy {
	case "normal":
		b, err = normal.New(config, ethereumAddress, pe, wc)
	default:
		err = errors.New("unrecognised bot strategy")
	}
	if err != nil {
		err = fmt.Errorf("failed to create new bot with strategy %s: %w", config.Strategy, err)
	}
	return
}
