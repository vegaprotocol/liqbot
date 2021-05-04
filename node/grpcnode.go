package node

import (
	"context"
	"fmt"
	"net/url"
	"time"

	e "code.vegaprotocol.io/liqbot/errors"

	"github.com/pkg/errors"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
	"google.golang.org/grpc"
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
		return nil, errors.Wrap(err, fmt.Sprintf("failed to dial gRPC node: %s", hostPort))
	}
	node.conn = conn
	return &node, nil
}

// GetAddress gets the address of the node.
func (n *GRPCNode) GetAddress() (url.URL, error) {
	if n == nil {
		return url.URL{}, errors.Wrap(e.ErrNil, "failed to get node address")
	}
	return n.address, nil
}

// === Trading ===

// SubmitTransaction submits a signed transaction
func (n *GRPCNode) SubmitTransaction(req *api.SubmitTransactionRequest) (resp *api.SubmitTransactionResponse, err error) {
	msg := "gRPC call failed: SubmitTransaction"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	resp, err = c.SubmitTransaction(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// === Trading Data ===

// GetVegaTime gets the latest block header time from the node.
func (n *GRPCNode) GetVegaTime() (t time.Time, err error) {
	msg := "gRPC call failed: GetVegaTime"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.GetVegaTime(ctx, &api.GetVegaTimeRequest{})
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	nsec := response.Timestamp
	if nsec < 0 {
		err = errors.Wrap(fmt.Errorf("negative time: %d", nsec), msg)
		return
	}
	t = time.Unix(0, nsec).UTC()
	return
}

// MarketByID gets a Market from the node
func (n *GRPCNode) MarketByID(req *api.MarketByIDRequest) (response *api.MarketByIDResponse, err error) {
	msg := "gRPC call failed: MarketByID"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketByID(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// MarketDataByID gets market data from the node
func (n *GRPCNode) MarketDataByID(req *api.MarketDataByIDRequest) (response *api.MarketDataByIDResponse, err error) {
	msg := "gRPC call failed: MarketDataByID"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketDataByID(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// LiquidityProvisions gets the liquidity provisions for a given market and party.
func (n *GRPCNode) LiquidityProvisions(req *api.LiquidityProvisionsRequest) (response *api.LiquidityProvisionsResponse, err error) {
	msg := "gRPC call failed: LiquidityProvisions"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.LiquidityProvisions(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// MarketDepth gets the depth for a market.
func (n *GRPCNode) MarketDepth(req *api.MarketDepthRequest) (response *api.MarketDepthResponse, err error) {
	msg := "gRPC call failed: MarketDepth"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.MarketDepth(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// PartyAccounts gets Accounts for a given partyID from the node
func (n *GRPCNode) PartyAccounts(req *api.PartyAccountsRequest) (response *api.PartyAccountsResponse, err error) {
	msg := "gRPC call failed: PartyAccounts"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.PartyAccounts(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// PositionsByParty gets the positions for a party.
func (n *GRPCNode) PositionsByParty(req *api.PositionsByPartyRequest) (response *api.PositionsByPartyResponse, err error) {
	msg := "gRPC call failed: PositionsByParty"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err = c.PositionsByParty(ctx, req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// ObserveEventBus starts a network connection to the node to sending event messages on
func (n *GRPCNode) ObserveEventBus() (stream api.TradingDataService_ObserveEventBusClient, err error) {
	msg := "gRPC call failed: ObserveEventBus"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	stream, err = c.ObserveEventBus(context.Background())
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}

// PositionsSubscribe starts a network connection to receive the party position as it updates
func (n *GRPCNode) PositionsSubscribe(req *api.PositionsSubscribeRequest) (stream api.TradingDataService_PositionsSubscribeClient, err error) {
	msg := "gRPC call failed: PositionsSubscribe"
	if n == nil {
		err = errors.Wrap(e.ErrNil, msg)
		return
	}

	c := api.NewTradingDataServiceClient(n.conn)
	stream, err = c.PositionsSubscribe(context.Background(), req)
	if err != nil {
		err = errors.Wrap(err, msg)
		return
	}
	return
}
