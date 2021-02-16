package pricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"code.vegaprotocol.io/liqbot/config"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
)

// Engine represents a pricing engine. Do not use this directly. Use New() and an interface.
type Engine struct {
	config config.PricingConfig
	client http.Client
}

// NewEngine creates a new pricing engine
func NewEngine(cfg config.PricingConfig) *Engine {
	e := Engine{
		config: cfg,
		client: http.Client{},
	}

	return &e
}

func (e *Engine) GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error) {
	v := url.Values{}
	if pricecfg.Source != "" {
		v.Set("source", pricecfg.Source)
	}
	if pricecfg.Source != "" {
		v.Set("base", pricecfg.Base)
	}
	if pricecfg.Source != "" {
		v.Set("quote", pricecfg.Quote)
	}
	if pricecfg.Source != "" {
		v.Set("wander", fmt.Sprintf("%v", pricecfg.Wander))
	}
	relativeURL := &url.URL{RawQuery: v.Encode()}
	fullURL := e.config.Address.ResolveReference(relativeURL).String()
	req, _ := http.NewRequest(http.MethodGet, fullURL, nil)

	var resp *http.Response
	resp, err = e.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad HTTP response code: %d", resp.StatusCode)
		return
	}

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var response ppservice.PricesResponse
	if err = json.Unmarshal(content, &response); err != nil {
		return
	}

	if len(response.Prices) == 0 {
		err = errors.New("zero-length price list from Price Proxy")
		return
	}

	return *response.Prices[0], nil
}
