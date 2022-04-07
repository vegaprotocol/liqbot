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

// NewEngine creates a new pricing engine.
func NewEngine(cfg config.PricingConfig) *Engine {
	e := Engine{
		config: cfg,
		client: http.Client{},
	}

	return &e
}

// GetPrice fetches a live/recent price from the price proxy.
func (e *Engine) GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error) {
	v := url.Values{}
	if pricecfg.Source != "" {
		v.Set("source", pricecfg.Source)
	}
	if pricecfg.Base != "" {
		v.Set("base", pricecfg.Base)
	}
	if pricecfg.Quote != "" {
		v.Set("quote", pricecfg.Quote)
	}
	v.Set("wander", fmt.Sprintf("%v", pricecfg.Wander))
	relativeURL := &url.URL{RawQuery: v.Encode()}
	fullURL := e.config.Address.ResolveReference(relativeURL).String()
	req, _ := http.NewRequest(http.MethodGet, fullURL, nil)

	var resp *http.Response
	resp, err = e.client.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to perform HTTP request: %w", err)
		return
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("failed to read HTTP response body: %w", err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad response: HTTP %d %s", resp.StatusCode, string(content))
		return
	}

	var response ppservice.PricesResponse
	if err = json.Unmarshal(content, &response); err != nil {
		err = fmt.Errorf("failed to parse HTTP response as JSON: %w", err)
		return
	}

	if len(response.Prices) == 0 {
		err = errors.New("zero-length price list from Price Proxy")
		return
	}

	return *response.Prices[0], nil
}
