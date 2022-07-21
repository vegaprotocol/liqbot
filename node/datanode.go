package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	e "code.vegaprotocol.io/liqbot/errors"
)

// DataNode stores state for a Vega Data node.
type DataNode struct {
	hosts       []string // format: host:port
	callTimeout time.Duration
	conn        *grpc.ClientConn
	connecting  bool
	mu          sync.RWMutex
	log         *log.Entry
}

// NewDataNode returns a new node.
func NewDataNode(hosts []string, callTimeout time.Duration) *DataNode {
	return &DataNode{
		hosts:       hosts,
		callTimeout: callTimeout,
		log:         log.WithFields(log.Fields{"service": "DataNode"}),
	}
}

func (n *DataNode) DialConnection() chan struct{} {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(len(n.hosts))
	waitCh := make(chan struct{})

	n.mu.Lock()
	if n.connecting {
		n.mu.Unlock()
		return waitCh
	}

	n.log.WithFields(
		log.Fields{
			"hosts": n.hosts,
		}).Debug("Attempting to connect to DataNode...")

	n.connecting = true
	n.mu.Unlock()

	for _, h := range n.hosts {
		go func(host string) {
			defer func() {
				cancel()
				wg.Done()
			}()
			n.dialNode(ctx, host)
		}(h)
	}

	wg.Wait()
	n.mu.Lock()
	n.connecting = false
	n.mu.Unlock()
	close(waitCh)
	return waitCh
}

func (n *DataNode) dialNode(ctx context.Context, host string) {
	conn, err := grpc.DialContext(
		ctx,
		host,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		n.mu.RLock()
		defer n.mu.RUnlock()
		// another goroutine has already established a connection
		if n.conn == nil {
			n.log.WithFields(
				log.Fields{
					"error": err,
					"host":  host,
				}).Error("Failed to connect to data node")
		}
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	n.conn = conn

	n.log.WithFields(
		log.Fields{
			"host":  host,
			"state": n.conn.GetState(),
		}).Info("Connected to data node")
	return
}

func (n *DataNode) Target() string {
	return n.conn.Target()
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
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

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
		err = fmt.Errorf(msg, e.ErrorDetail(err))
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
		err = fmt.Errorf(msg, e.ErrorDetail(err))
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
		err = fmt.Errorf(msg, e.ErrorDetail(err))
		return
	}
	return
}

// === TradingDataService ===

// PartyAccounts returns accounts for the given party.
func (n *DataNode) PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error) {
	msg := "gRPC call failed (data-node): PartyAccounts: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.PartyAccounts(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

// MarketDataByID returns market data for the specified market.
func (n *DataNode) MarketDataByID(req *dataapipb.MarketDataByIDRequest) (response *dataapipb.MarketDataByIDResponse, err error) {
	msg := "gRPC call failed (data-node): MarketDataByID: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.MarketDataByID(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

// Markets returns all markets.
func (n *DataNode) Markets(req *dataapipb.MarketsRequest) (response *dataapipb.MarketsResponse, err error) {
	msg := "gRPC call failed (data-node): Markets: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.Markets(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

// PositionsByParty returns positions for the given party.
func (n *DataNode) PositionsByParty(req *dataapipb.PositionsByPartyRequest) (response *dataapipb.PositionsByPartyResponse, err error) {
	msg := "gRPC call failed (data-node): PositionsByParty: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.PositionsByParty(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

// PositionsSubscribe opens a stream.
func (n *DataNode) PositionsSubscribe(req *dataapipb.PositionsSubscribeRequest) (client dataapipb.TradingDataService_PositionsSubscribeClient, err error) {
	msg := "gRPC call failed: PositionsSubscribe: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	// no timeout on streams
	client, err = c.PositionsSubscribe(context.Background(), req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
		return
	}
	return
}

// AssetByID returns the specified asset.
func (n *DataNode) AssetByID(req *dataapipb.AssetByIDRequest) (response *dataapipb.AssetByIDResponse, err error) {
	msg := "gRPC call failed (data-node): AssetByID: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := dataapipb.NewTradingDataServiceClient(n.conn)
	ctx, cancel := context.WithTimeout(context.Background(), n.callTimeout)
	defer cancel()

	response, err = c.AssetByID(ctx, req)
	if err != nil {
		err = fmt.Errorf(msg, e.ErrorDetail(err))
	}
	return
}

func (n *DataNode) WaitForStateChange(ctx context.Context, state connectivity.State) bool {
	return n.conn.WaitForStateChange(ctx, state)
}
