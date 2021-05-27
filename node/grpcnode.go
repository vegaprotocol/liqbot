package node

import (
	"context"
	"fmt"
	"net/url"
	"time"

	e "code.vegaprotocol.io/liqbot/errors"

	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	apigrpc "github.com/vegaprotocol/api/grpc/clients/go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// GRPCNode stores state for a Vega node.
type GRPCNode struct {
	address     url.URL // format: host:port
	callTimeout time.Duration

	conn *grpc.ClientConn
}

// NewGRPCNode returns a new node.
func NewGRPCNode(addr url.URL, connectTimeout time.Duration, callTimeout time.Duration) (*GRPCNode, error) {
	node := GRPCNode{
		address:     addr,
		callTimeout: callTimeout,
	}

	hostPort := fmt.Sprintf("%s:%s", addr.Hostname(), addr.Port())
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, hostPort, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to dial gRPC node %s: %w", hostPort, err)
	}
	node.conn = conn
	return &node, nil
}

// GetAddress gets the address of the node.
func (n *GRPCNode) GetAddress() (url.URL, error) {
	if n == nil {
		return url.URL{}, fmt.Errorf("failed to get node address: %w", e.ErrNil)
	}
	return n.address, nil
}

// === Trading ===

// SubmitTransaction submits a signed transaction
func (n *GRPCNode) SubmitTransaction(req *api.SubmitTransactionRequest) (response *api.SubmitTransactionResponse, err error) {
	msg := "gRPC call failed: SubmitTransaction: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.SubmitTransaction(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// === Trading Data ===

// GetVegaTime gets the latest block header time from the node.
func (n *GRPCNode) GetVegaTime() (t time.Time, err error) {
	msg := "gRPC call failed: GetVegaTime: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.GetVegaTime(ctx, &api.GetVegaTimeRequest{})
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
		return
	}
	nsec := response.Timestamp
	if nsec < 0 {
		err = fmt.Errorf("negative time: %d", nsec)
		err = fmt.Errorf(msg, err)
		return
	}
	t = time.Unix(0, nsec).UTC()
	return
}

// MarketByID gets a Market from the node
func (n *GRPCNode) MarketByID(req *api.MarketByIDRequest) (response *api.MarketByIDResponse, err error) {
	msg := "gRPC call failed: MarketByID: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketByID(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// MarketDataByID gets market data from the node
func (n *GRPCNode) MarketDataByID(req *api.MarketDataByIDRequest) (response *api.MarketDataByIDResponse, err error) {
	msg := "gRPC call failed: MarketDataByID: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketDataByID(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// LiquidityProvisions gets the liquidity provisions for a given market and party.
func (n *GRPCNode) LiquidityProvisions(req *api.LiquidityProvisionsRequest) (response *api.LiquidityProvisionsResponse, err error) {
	msg := "gRPC call failed: LiquidityProvisions: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.LiquidityProvisions(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// MarketDepth gets the depth for a market.
func (n *GRPCNode) MarketDepth(req *api.MarketDepthRequest) (response *api.MarketDepthResponse, err error) {
	msg := "gRPC call failed: MarketDepth: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketDepth(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// PartyAccounts gets Accounts for a given partyID from the node
func (n *GRPCNode) PartyAccounts(req *api.PartyAccountsRequest) (response *api.PartyAccountsResponse, err error) {
	msg := "gRPC call failed: PartyAccounts: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.PartyAccounts(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// PositionsByParty gets the positions for a party.
func (n *GRPCNode) PositionsByParty(req *api.PositionsByPartyRequest) (response *api.PositionsByPartyResponse, err error) {
	msg := "gRPC call failed: PositionsByParty: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.PositionsByParty(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// ObserveEventBus starts a network connection to the node to sending event messages on
func (n *GRPCNode) ObserveEventBus() (stream api.TradingDataService_ObserveEventBusClient, err error) {
	msg := "gRPC call failed: ObserveEventBus: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	stream, err = c.ObserveEventBus(context.Background())
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// PositionsSubscribe starts a network connection to receive the party position as it updates
func (n *GRPCNode) PositionsSubscribe(req *api.PositionsSubscribeRequest) (stream api.TradingDataService_PositionsSubscribeClient, err error) {
	msg := "gRPC call failed: PositionsSubscribe: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	stream, err = c.PositionsSubscribe(context.Background(), req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}

// AssetByID returns information about a given asset
func (n *GRPCNode) AssetByID(assetID string) (response *api.AssetByIDResponse, err error) {
	msg := "gRPC call failed: AssetByID: %w"
	if n == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	req := &api.AssetByIDRequest{
		Id: assetID,
	}
	response, err = c.AssetByID(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, apigrpc.ErrorDetail(err))
	}
	return
}
