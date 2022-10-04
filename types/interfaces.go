package types

import (
	"context"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	v1 "code.vegaprotocol.io/vega/protos/vega/events/v1"

	"code.vegaprotocol.io/liqbot/types/num"
)

// Bot is the generic bot interface.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/bot_mock.go -package mocks code.vegaprotocol.io/liqbot/bot Bot
type Bot interface {
	Start() error
	Stop()
	GetTraderDetails() string
}

// PricingEngine is the source of price information from the price proxy.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

type CoinProvider interface {
	TopUpAsync(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) (v1.BusEventType, error)
	StakeAsync(ctx context.Context, receiverAddress, assetID string, amount *num.Uint) error
}
