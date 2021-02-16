package proto_test

import (
	"testing"

	restproto "code.vegaprotocol.io/liqbot/proto"

	"github.com/stretchr/testify/assert"
)

func TestConvertMarket(t *testing.T) {
	const (
		id = "someid"
	)

	cm, err := restproto.ConvertMarket(nil)
	assert.NoError(t, err)
	assert.Nil(t, cm)

	rm := restproto.RESTMarket{
		ID:            id,
		DecimalPlaces: "0",
	}
	m, err := restproto.ConvertMarket(&rm)
	assert.NoError(t, err)
	assert.Equal(t, id, m.Id)

	rm.TradableInstrument = &restproto.RESTTradableInstrument{}
	m, err = restproto.ConvertMarket(&rm)
	assert.NoError(t, err)
	assert.Nil(t, m.TradableInstrument.Instrument)

	rm.TradableInstrument.Instrument = &restproto.RESTInstrument{
		ID:   "an_id",
		Code: "a_code",
		Name: "a_name",
		Metadata: &restproto.RESTInstrumentMetadata{
			Tags: []string{"fx/base:ABC", "fx/quote:XYZ"},
		},
	}
	m, err = restproto.ConvertMarket(&rm)
	assert.NoError(t, err)
	assert.Equal(t, rm.TradableInstrument.Instrument.ID, m.TradableInstrument.Instrument.Id)
}
