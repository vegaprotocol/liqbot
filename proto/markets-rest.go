package proto

import (
	"strconv"

	p "github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
)

// RESTInstrumentMetadata is a RESTified InstrumentMetadata.
type RESTInstrumentMetadata struct {
	Tags []string `json:"tags"`
}

// RESTInstrument is a RESTified Instrument.
type RESTInstrument struct {
	ID       string                  `json:"id"`
	Code     string                  `json:"code"`
	Name     string                  `json:"name"`
	Metadata *RESTInstrumentMetadata `json:"metadata"`
}

// RESTTradableInstrument is a RESTified TradableInstrument.
type RESTTradableInstrument struct {
	Instrument *RESTInstrument `json:"instrument"`
}

// RESTMarket is a RESTified Market.
type RESTMarket struct {
	ID                 string                  `json:"id"`
	Name               string                  `json:"name"`
	DecimalPlaces      string                  `json:"decimalPlaces"` // uint64
	TradableInstrument *RESTTradableInstrument `json:"tradableInstrument"`
}

// ConvertInstrument converts a RESTInstrument into an Instrument
func ConvertInstrument(i *RESTInstrument) *p.Instrument {
	if i == nil {
		return nil
	}
	pi := p.Instrument{
		Id:   i.ID,
		Code: i.Code,
		Name: i.Name,
	}
	if i.Metadata != nil {
		pi.Metadata = &p.InstrumentMetadata{
			Tags: i.Metadata.Tags,
		}
	}
	return &pi
}

// ConvertTradableInstrument converts a RESTTradableInstrument into a TradableInstrument
func ConvertTradableInstrument(ti *RESTTradableInstrument) *p.TradableInstrument {
	if ti == nil {
		return nil
	}
	return &p.TradableInstrument{
		Instrument: ConvertInstrument(ti.Instrument),
	}
}

// ConvertMarket converts a RESTMarket into a Market
func ConvertMarket(m *RESTMarket) (*p.Market, error) {
	if m == nil {
		return nil, nil
	}
	decimalPlaces, err := strconv.ParseUint(m.DecimalPlaces, 10, 64)
	if err != nil {
		return nil, err
	}
	return &p.Market{
		Id:                 m.ID,
		DecimalPlaces:      decimalPlaces,
		TradableInstrument: ConvertTradableInstrument(m.TradableInstrument),
	}, nil
}
