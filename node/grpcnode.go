package node

import (
	"context"
	"fmt"
	"net/url"
	"time"

	e "code.vegaprotocol.io/liqbot/errors"

	"github.com/pkg/errors"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto/api"
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

// // PrepareSubmitOrder prepares a SubmitOrder request so it can be sined and submitted to SubmitTransaction.
// func (n *GRPCNode) PrepareSubmitOrder(req *api.PrepareSubmitOrderRequest) (*api.PrepareSubmitOrderResponse, error) {
// 	if n == nil {
// 		return nil, ErrNil
// 	}

// 	c := api.NewTradingServiceClient(n.conn)
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// 	defer cancel()
// 	return c.PrepareSubmitOrder(ctx, req)
// }

// // SubmitTransaction submits a signed transaction
// func (n *GRPCNode) SubmitTransaction(req *api.SubmitTransactionRequest) (resp *api.SubmitTransactionResponse, err error) {
// 	if n == nil {
// 		return nil, ErrNil
// 	}

// 	c := api.NewTradingServiceClient(n.conn)
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// 	defer cancel()
// 	return c.SubmitTransaction(ctx, req)
// }

// // UpdateMarketDepths gets the latest price for each market.
// func (n *GRPCNode) UpdateMarketDepths() error {
// 	if n == nil {
// 		return ErrNil
// 	}

// 	results := make(map[string]proto.MarketDepth)
// 	c := api.NewTradingDataServiceClient(n.conn)
// 	n.marketsMu.Lock()
// 	defer n.marketsMu.Unlock()
// 	for mktID := range n.markets {
// 		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
// 		defer cancel()

// 		mdr, err := c.MarketDepth(ctx, &api.MarketDepthRequest{MarketId: mktID})
// 		if err != nil {
// 			return err
// 		}

// 		results[mktID] = *MarketDepthFromMarketDepthResponse(mdr)
// 	}

// 	n.marketDepthsMu.Lock()
// 	n.marketDepths = results
// 	n.marketDepthsMu.Unlock()
// 	return nil
// }

// === Trading ===

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
