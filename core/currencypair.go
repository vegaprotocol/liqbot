package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
)

var currencyPairBaseTags = []string{
	"base:",
	"ticker:",
}

var currencyPairQuoteTags = []string{
	"quote:",
}

var currencyPairCountryTags = []string{
	"country:",
}

var countryToCurrency = map[string]string{
	"US": "USD",
}

// MarketToPriceConfig converts a Market object to a PriceConfig object by looking in the Market's instrument metadata tag list.
func MarketToPriceConfig(mkt *proto.Market) (*ppconfig.PriceConfig, error) {
	if mkt == nil || mkt.TradableInstrument == nil || mkt.TradableInstrument.Instrument == nil ||
		mkt.TradableInstrument.Instrument.Metadata == nil {
		return nil, errors.New("failed to create PriceConfig from Market (nil)")
	}
	if len(mkt.TradableInstrument.Instrument.Metadata.Tags) == 0 {
		return nil, errors.New("failed to create PriceConfig from Market (zero Tags)")
	}
	base := "?"
	quote := "?"
	sort.Strings(mkt.TradableInstrument.Instrument.Metadata.Tags)
	for _, tag := range mkt.TradableInstrument.Instrument.Metadata.Tags {
		for _, pfx := range currencyPairBaseTags {
			if strings.HasPrefix(tag, pfx) {
				base = tag[len(pfx):]
			}
		}
		for _, pfx := range currencyPairQuoteTags {
			if strings.HasPrefix(tag, pfx) {
				quote = tag[len(pfx):]
			}
		}
		for _, pfx := range currencyPairCountryTags {
			if strings.HasPrefix(tag, pfx) {
				var found bool
				quote, found = countryToCurrency[tag[len(pfx):]]
				if !found {
					return nil, fmt.Errorf("can't map country to currency: %s", tag[len(pfx):])
				}
			}
		}
	}
	if base == "?" {
		return nil, errors.New("failed to create PriceConfig from Market (missing Base)")
	}
	if quote == "?" {
		return nil, errors.New("failed to create PriceConfig from Market (missing Quote)")
	}

	return &ppconfig.PriceConfig{Base: base, Quote: quote}, nil
}
