package node

import (
	"context"
	"fmt"
	"net/url"
	"time"

	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/helpers"

	dataapipbv2 "code.vegaprotocol.io/vega/protos/data-node/api/v2"

	vegaapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

var (
	ErrFailedCreateTradingServiceClient = fmt.Errorf("failed to create new trading service client")

	ErrMsgFailedTradingServiceRequest = "failed request to the trading service client %s service: %w"
	ErrMsgCheckConnection             = "failed connecting to the data-base: %w"
)

// DataNode stores state for a Vega Data node.
type DataNode struct {
	address     url.URL // format: host:port
	callTimeout time.Duration

	conn *grpc.ClientConn
}

// NewDataNode returns a new node.
func NewDataNode(addr url.URL, connectTimeout time.Duration, callTimeout time.Duration) (*DataNode, error) {
	node := DataNode{
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
func (n *DataNode) GetAddress() (url.URL, error) {
	if n == nil {
		return url.URL{}, fmt.Errorf("failed to get node address: %w", e.ErrNil)
	}
	return n.address, nil
}

// === CoreService ===

// SubmitTransaction submits a signed v2 transaction.
func (n *DataNode) SubmitTransaction(req *vegaapipb.SubmitTransactionRequest) (response *vegaapipb.SubmitTransactionResponse, err error) {
	msg := "gRPC call failed: SubmitTransaction: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.SubmitTransaction(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc PropagateChainEvent(PropagateChainEventRequest) returns (PropagateChainEventResponse);
// rpc Statistics(StatisticsRequest) returns (StatisticsResponse);

// LastBlockData gets the latest blockchain data, height, hash and pow parameters.
func (n *DataNode) LastBlockData() (*vegaapipb.LastBlockHeightResponse, error) {
	msg := "gRPC call failed: LastBlockData: %w"
	if n == nil {
		return nil, fmt.Errorf(msg, e.ErrNil)
	}

	if n.conn.GetState() != connectivity.Ready {
		return nil, fmt.Errorf(msg, e.ErrConnectionNotReady)
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	var response *vegaapipb.LastBlockHeightResponse
	response, err := c.LastBlockHeight(ctx, &vegaapipb.LastBlockHeightRequest{})
	if err != nil {
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return response, err
}

// GetVegaTime gets the latest block header time from the node.
func (n *DataNode) GetVegaTime() (t time.Time, err error) {
	msg := "gRPC call failed: GetVegaTime: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.GetVegaTime(ctx, &vegaapipb.GetVegaTimeRequest{})
	if err != nil {
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
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

// ObserveEventBus opens a stream.
func (n *DataNode) ObserveEventBus() (client vegaapipb.CoreService_ObserveEventBusClient, err error) {
	msg := "gRPC call failed: ObserveEventBus: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	// no timeout on streams
	client, err = c.ObserveEventBus(context.Background())
	if err != nil {
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
		return
	}
	// client.Send(req)
	// client.CloseSend()
	return
}

// === TradingDataService ===

// rpc MarketAccounts(MarketAccountsRequest) returns (MarketAccountsResponse);

func (n *DataNode) CheckConnection() error {
	if n == nil {
		return fmt.Errorf("data-node connection is nil: %w", e.ErrNil)
	}

	if n.conn.GetState() != connectivity.Ready {
		return fmt.Errorf("data-node-connection is not ready: %w", e.ErrConnectionNotReady)
	}

	return nil
}

func (n *DataNode) ListAccounts(req *dataapipbv2.ListAccountsRequest) (*dataapipbv2.ListAccountsResponse, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.ListAccounts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "list account", err)
	}

	return response, nil
}

// func (n *DataNode) GetMarket(req *dataapipbv2.GetMarketRequest) (*dataapipbv2.GetMarketResponse, error) {
// 	if err := n.CheckConnection(); err != nil {
// 		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
// 	}

// 	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
// 	if c == nil {
// 		return nil, ErrFailedCreateTradingServiceClient
// 	}

// 	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
// 	defer cancel()
// 	response, err := c.GetMarket(ctx, req)
// 	if err != nil {
// 		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "get market", err)
// 	}

// 	return response, nil
// }

func (n *DataNode) GetLatestMarketData(req *dataapipbv2.GetLatestMarketDataRequest) (*dataapipbv2.GetLatestMarketDataResponse, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.GetLatestMarketData(ctx, req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "get market", err)
	}

	return response, nil
}

func (n *DataNode) ListMarkets(req *dataapipbv2.ListMarketsRequest) (*dataapipbv2.ListMarketsResponse, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.ListMarkets(ctx, req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "list markets", err)
	}

	return response, nil
}

func (n *DataNode) ListPositions(req *dataapipbv2.ListPositionsRequest) (*dataapipbv2.ListPositionsResponse, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.ListPositions(ctx, req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "list positions", err)
	}

	return response, nil
}

func (n *DataNode) ObservePositions(req *dataapipbv2.ObservePositionsRequest) (dataapipbv2.TradingDataService_ObservePositionsClient, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	client, err := c.ObservePositions(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "observe positions", err)
	}

	return client, nil
}

func (n *DataNode) ListAssets(req *dataapipbv2.ListAssetsRequest) (*dataapipbv2.ListAssetsResponse, error) {
	if err := n.CheckConnection(); err != nil {
		return nil, fmt.Errorf(ErrMsgCheckConnection, err)
	}

	c := dataapipbv2.NewTradingDataServiceClient(n.conn)
	if c == nil {
		return nil, ErrFailedCreateTradingServiceClient
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()
	response, err := c.ListAssets(ctx, req)
	if err != nil {
		return nil, fmt.Errorf(ErrMsgFailedTradingServiceRequest, "list assets", err)
	}

	return response, nil
}

func (n *DataNode) Statistics(*vegaapipb.StatisticsRequest) (*vegaapipb.StatisticsResponse, error) {
	if n == nil {
		return nil, fmt.Errorf("data node instance is nil: %w", e.ErrNil)
	}

	if n.conn.GetState() != connectivity.Ready {
		return nil, fmt.Errorf("grpc connection for data node is not ready: %w", e.ErrConnectionNotReady)
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err := c.Statistics(ctx, &vegaapipb.StatisticsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node statistics: %w", err)
	}

	return response, nil
}
