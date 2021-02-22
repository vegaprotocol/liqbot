package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	restproto "code.vegaprotocol.io/liqbot/proto"
	restprotoapi "code.vegaprotocol.io/liqbot/proto/api"

	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

const (
	urlFountain            = "fountain"
	urlMarkets             = "markets"
	urlMarketDepth         = "markets/%s/depth"
	urlOrders              = "orders"
	urlOrdersPrepareSubmit = "orders/prepare/submit"
	urlTime                = "time"
	urlTransaction         = "transaction"
)

// RESTNode stores state for a Vega node which communicates with REST.
type RESTNode struct {
	Name    string `json:"name"`
	Address url.URL

	// These elements are protected by the mutex at the end of the block
	httpClient *http.Client
	logger     *log.Entry
	time       time.Time // VegaTime
	mu         sync.Mutex

	// marketDepths is protected by a separate mutex
	marketDepths   map[string]proto.MarketDepth // MarketID -> MarketDepth
	marketDepthsMu sync.Mutex

	// markets is protected by a separate mutex
	markets   map[string]proto.Market // MarketID -> Market
	marketsMu sync.Mutex
}

// NewRESTNode returns a new node.
func NewRESTNode(ctx context.Context, name string, addr url.URL) (Node, error) {
	node := RESTNode{
		Name:    name,
		Address: addr,

		logger:       log.WithFields(log.Fields{"node": name}),
		httpClient:   NewHTTPClient(),
		marketDepths: make(map[string]proto.MarketDepth),
		markets:      make(map[string]proto.Market),
	}

	err := waitForNode(ctx, &node)
	if err != nil {
		return nil, err
	}

	go updateNode(&node)

	return &node, nil
}

// NilRestNode returns nil. Used in testing only.
func NilRestNode() (*RESTNode, error) {
	var n *RESTNode
	return n, nil
}

// GetAddress gets the address of the node.
func (n *RESTNode) GetAddress() (url.URL, error) {
	if n == nil {
		return url.URL{}, ErrNil
	}
	return n.Address, nil
}

// GetMarket gets a single Market of the node.
func (n *RESTNode) GetMarket(marketID string) (proto.Market, error) {
	if n == nil {
		return proto.Market{}, ErrNil
	}
	n.marketsMu.Lock()
	defer n.marketsMu.Unlock()

	if mkt, present := n.markets[marketID]; present {
		return mkt, nil
	}

	return proto.Market{}, ErrMarketNotFound
}

// GetMarketDepth gets the market depth for one of the node's Markets.
func (n *RESTNode) GetMarketDepth(marketID string) (proto.MarketDepth, error) {
	if n == nil {
		return proto.MarketDepth{}, ErrNil
	}
	n.marketDepthsMu.Lock()
	md, found := n.marketDepths[marketID]
	n.marketDepthsMu.Unlock()
	if !found {
		return proto.MarketDepth{}, ErrMarketNotFound
	}
	return md, nil
}

// GetMarketDepths gets the market depth for all of the node's Markets.
func (n *RESTNode) GetMarketDepths() (map[string]proto.MarketDepth, error) {
	if n == nil {
		return nil, ErrNil
	}
	results := make(map[string]proto.MarketDepth)
	n.marketDepthsMu.Lock()
	for id, md := range n.marketDepths {
		results[id] = md
	}
	n.marketDepthsMu.Unlock()
	return results, nil
}

// GetMarkets gets the Markets of the node.
func (n *RESTNode) GetMarkets() (map[string]proto.Market, error) {
	if n == nil {
		return nil, ErrNil
	}
	results := make(map[string]proto.Market)
	n.marketsMu.Lock()
	for id, market := range n.markets {
		results[id] = market
	}
	n.marketsMu.Unlock()
	return results, nil
}

// GetName gets the name of the node.
func (n *RESTNode) GetName() (string, error) {
	if n == nil {
		return "", ErrNil
	}
	return n.Name, nil
}

// GetTime gets the blockchain time of the node.
func (n *RESTNode) GetTime() (time.Time, error) {
	if n == nil {
		return time.Time{}, ErrNil
	}
	n.mu.Lock()
	t := n.time
	n.mu.Unlock()
	return t, nil
}

// PrepareSubmitOrder prepares a SubmitOrder request so it can be sined and submitted to SubmitTransaction.
func (n *RESTNode) PrepareSubmitOrder(req *api.PrepareSubmitOrderRequest) (*api.PrepareSubmitOrderResponse, error) {
	if n == nil {
		return nil, ErrNil
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	content, err := DoHTTP(n.httpClient, n.url(urlOrdersPrepareSubmit), http.MethodPost, bytes.NewBuffer(payload), nil, true)
	if err != nil {
		return nil, err
	}

	resp := api.PrepareSubmitOrderResponse{}
	if err := json.Unmarshal(content, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// SubmitTransaction submits a signed transaction
func (n *RESTNode) SubmitTransaction(req *api.SubmitTransactionRequest) (*api.SubmitTransactionResponse, error) {
	if n == nil {
		return nil, ErrNil
	}

	restReq, err := restprotoapi.ConvertSubmitTransactionRequest(req)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(restReq)
	if err != nil {
		return nil, err
	}

	content, err := DoHTTP(n.httpClient, n.url(urlTransaction), http.MethodPost, bytes.NewBuffer(payload), nil, true)
	if err != nil {
		return nil, err
	}

	resp := api.SubmitTransactionResponse{}
	if err := json.Unmarshal(content, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// UpdateMarkets gets the list of Markets from the node
func (n *RESTNode) UpdateMarkets() error {
	if n == nil {
		return ErrNil
	}

	content, err := DoHTTP(n.httpClient, n.url(urlMarkets), http.MethodGet, nil, nil, true)
	if err != nil {
		return err
	}

	failedToGetMarkets := ErrFailedToGetMarkets.Error()
	marketsResponse := restprotoapi.RESTMarketsResponse{}
	if err = json.Unmarshal(content, &marketsResponse); err != nil {
		n.logger.WithFields(log.Fields{
			"error":            err.Error(),
			"response_content": string(content),
		}).Debug(failedToGetMarkets)
		return err
	}

	results := make(map[string]proto.Market)
	for _, market := range marketsResponse.Markets {
		mkt, err := restproto.ConvertMarket(market)
		if err != nil {
			return err
		}
		results[market.ID] = *mkt
	}

	n.marketsMu.Lock()
	n.markets = results
	n.marketsMu.Unlock()
	return nil
}

// UpdateTime gets the latest block header time from the node.
func (n *RESTNode) UpdateTime() error {
	if n == nil {
		return ErrNil
	}

	failedToGetVegaTime := ErrFailedToGetVegaTime.Error()

	content, err := DoHTTP(n.httpClient, n.url(urlTime), http.MethodGet, nil, nil, true)
	if err != nil {
		return err
	}

	vegaTime := restprotoapi.RESTGetVegaTimeResponse{}
	if err = json.Unmarshal(content, &vegaTime); err != nil {
		n.logger.WithFields(log.Fields{
			"response_content": string(content),
		}).Warning(failedToGetVegaTime)
		return err
	}

	vt, err := restprotoapi.ConvertGetVegaTimeResponse(&vegaTime)
	if err != nil {
		n.logger.WithFields(log.Fields{
			"vegatime_string": vegaTime.Timestamp,
		}).Warning(failedToGetVegaTime)
		return err
	}
	if vt.Timestamp < 0 {
		n.logger.WithFields(log.Fields{
			"nsec": vt.Timestamp,
		}).Debug(failedToGetVegaTime)
		return ErrNegativeTime
	}
	n.mu.Lock()
	n.time = time.Unix(0, vt.Timestamp).UTC()
	n.mu.Unlock()
	return nil
}

func decodeGrpcStatus(content []byte) string {
	grpcStatus := struct {
		// https://github.com/grpc/grpc/blob/master/src/proto/grpc/status/status.proto#L80
		Code    int
		Message string
		Details []*proto.ErrorDetail
	}{}
	if err := json.Unmarshal(content, &grpcStatus); err != nil {
		return fmt.Sprintf("could not decode gRPC status: %v", err)
	}

	b := &strings.Builder{}

	fmt.Fprintf(b, "gRPC error: code:%d message:\"%s\"", grpcStatus.Code, grpcStatus.Message)
	for _, d := range grpcStatus.Details {
		fmt.Fprintf(b, "; Detail: code=%d message=\"%s\" inner=\"%s\"", d.Code, d.Message, d.Inner)
	}
	return b.String()
}

func (n *RESTNode) url(path string) *url.URL {
	return n.Address.ResolveReference(&url.URL{Path: path})
}

// UpdateMarketDepths gets the latest price for each market.
func (n *RESTNode) UpdateMarketDepths() error {
	if n == nil {
		return ErrNil
	}
	n.mu.Lock()
	defer n.mu.Unlock()

	results := make(map[string]proto.MarketDepth)
	for mktID := range n.markets {
		content, err := DoHTTP(n.httpClient, n.url(fmt.Sprintf(urlMarketDepth, mktID)), http.MethodGet, nil, nil, true)
		if err != nil {
			return err
		}
		depthResponse := restprotoapi.RESTMarketDepthResponse{}
		if err = json.Unmarshal(content, &depthResponse); err != nil {
			return err
		}
		mdr, err := restprotoapi.ConvertMarketDepthResponse(&depthResponse)
		if err != nil {
			return err
		}
		results[mktID] = *MarketDepthFromMarketDepthResponse(mdr)
	}

	n.marketDepthsMu.Lock()
	n.marketDepths = results
	n.marketDepthsMu.Unlock()
	return nil
}
