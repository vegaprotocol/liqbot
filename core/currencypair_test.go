package core_test

import (
	"testing"

	"code.vegaprotocol.io/liqbot/core"

	"code.vegaprotocol.io/vega/proto"
	"github.com/stretchr/testify/assert"
)

func TestMarketToPriceConfig(t *testing.T) {
	var mkt *proto.Market = nil
	cp, err := core.MarketToPriceConfig(mkt)
	assert.Nil(t, cp)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (nil)")

	mkt = &proto.Market{}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (nil)")

	mkt.TradableInstrument = &proto.TradableInstrument{}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (nil)")

	mkt.TradableInstrument.Instrument = &proto.Instrument{}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)

	mkt.TradableInstrument.Instrument.Metadata = &proto.InstrumentMetadata{}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (zero Tags)")

	mkt.TradableInstrument.Instrument.Metadata.Tags = []string{"somekey:somevalue"}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (missing Base)")

	mkt.TradableInstrument.Instrument.Metadata.Tags = []string{"base:ABC"}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.Error(t, err)
	assert.Nil(t, cp)
	assert.Contains(t, err.Error(), "failed to create PriceConfig from Market (missing Quote)")

	mkt.TradableInstrument.Instrument.Metadata.Tags = []string{"base:ABC", "quote:XYZ"}
	cp, err = core.MarketToPriceConfig(mkt)
	assert.NoError(t, err)
	if assert.NotNil(t, cp) {
		assert.Equal(t, "ABC", cp.Base)
		assert.Equal(t, "XYZ", cp.Quote)
	}
}
