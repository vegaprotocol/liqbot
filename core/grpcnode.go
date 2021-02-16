package core

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"code.vegaprotocol.io/vega/proto"
	"code.vegaprotocol.io/vega/proto/api"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// GRPCNode stores state for a Vega node.
type GRPCNode struct {
	Name    string `json:"name"`
	Address string // format: host:port

	// These elements are protected by the mutex at the end of the block
	conn   *grpc.ClientConn
	logger *log.Entry
	time   time.Time // VegaTime
	mu     sync.Mutex

	// marketDepths is protected by a separate mutex
	marketDepths   map[string]proto.MarketDepth // MarketID -> MarketDepth
	marketDepthsMu sync.Mutex

	// markets is protected by a separate mutex
	markets   map[string]proto.Market // MarketID -> Market
	marketsMu sync.Mutex
}

// NewGRPCNode returns a new node.
func NewGRPCNode(ctx context.Context, name string, addr url.URL) (Node, error) {
	node := GRPCNode{
		Name:    name,
		Address: fmt.Sprintf("%s:%s", addr.Hostname(), addr.Port()),

		logger:       log.WithFields(log.Fields{"node": name}),
		marketDepths: make(map[string]proto.MarketDepth),
		markets:      make(map[string]proto.Market),
	}

	conn, err := grpc.DialContext(ctx, node.Address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	node.conn = conn

	err = waitForNode(ctx, &node)
	if err != nil {
		return nil, err
	}

	go updateNode(&node)

	return &node, nil
}

// NilGrpcNode returns nil. Used in testing only.
func NilGrpcNode() (*GRPCNode, error) {
	var n *GRPCNode
	return n, nil
}

// GetAddress gets the address of the node.
func (n *GRPCNode) GetAddress() (url.URL, error) {
	if n == nil {
		return url.URL{}, ErrNil
	}
	return url.URL{Scheme: "grpc", Host: n.Address}, nil
}

// GetMarket gets a single Market of the node.
func (n *GRPCNode) GetMarket(marketID string) (proto.Market, error) {
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
func (n *GRPCNode) GetMarketDepth(marketID string) (proto.MarketDepth, error) {
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

// GetMarketDepths gets the market depth of the node's Markets.
func (n *GRPCNode) GetMarketDepths() (map[string]proto.MarketDepth, error) {
	if n == nil {
		return nil, ErrNil
	}
	results := make(map[string]proto.MarketDepth)
	n.marketDepthsMu.Lock()
	for id, candle := range n.marketDepths {
		results[id] = candle
	}
	n.marketDepthsMu.Unlock()
	return results, nil
}

// GetMarkets gets the Markets of the node.
func (n *GRPCNode) GetMarkets() (map[string]proto.Market, error) {
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
func (n *GRPCNode) GetName() (string, error) {
	if n == nil {
		return "", ErrNil
	}
	return n.Name, nil
}

// GetTime gets the blockchain time of the node.
func (n *GRPCNode) GetTime() (time.Time, error) {
	if n == nil {
		return time.Time{}, ErrNil
	}
	n.mu.Lock()
	t := n.time
	n.mu.Unlock()
	return t, nil
}

func (n *GRPCNode) PrepareSubmitOrder(req *api.PrepareSubmitOrderRequest) (*api.PrepareSubmitOrderResponse, error) {
	if n == nil {
		return nil, ErrNil
	}

	c := api.NewTradingServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return c.PrepareSubmitOrder(ctx, req)
}

// SubmitTransaction submits a signed transaction
func (n *GRPCNode) SubmitTransaction(req *api.SubmitTransactionRequest) (resp *api.SubmitTransactionResponse, err error) {
	if n == nil {
		return nil, ErrNil
	}

	c := api.NewTradingServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return c.SubmitTransaction(ctx, req)
}

// UpdateMarkets gets the list of Markets from the node
func (n *GRPCNode) UpdateMarkets() error {
	if n == nil {
		return ErrNil
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := c.Markets(ctx, &api.MarketsRequest{})
	if err != nil {
		return err
	}
	results := make(map[string]proto.Market)
	for _, market := range response.Markets {
		results[market.Id] = *market
	}

	n.marketsMu.Lock()
	n.markets = results
	n.marketsMu.Unlock()
	return nil
}

// UpdateTime gets the latest block header time from the node.
func (n *GRPCNode) UpdateTime() error {
	if n == nil {
		return ErrNil
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := c.GetVegaTime(ctx, &api.GetVegaTimeRequest{})
	if err != nil {
		return err
	}
	nsec := response.Timestamp
	if nsec < 0 {
		return ErrNegativeTime
	}
	n.mu.Lock()
	n.time = time.Unix(0, nsec).UTC()
	n.mu.Unlock()
	return nil
}

// UpdateMarketDepths gets the latest price for each market.
func (n *GRPCNode) UpdateMarketDepths() error {
	if n == nil {
		return ErrNil
	}

	results := make(map[string]proto.MarketDepth)
	c := api.NewTradingDataServiceClient(n.conn)
	n.marketsMu.Lock()
	defer n.marketsMu.Unlock()
	for mktID := range n.markets {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		mdr, err := c.MarketDepth(ctx, &api.MarketDepthRequest{MarketId: mktID})
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
