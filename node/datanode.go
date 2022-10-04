package node

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/vega/protos/vega/api/v1"

	e "code.vegaprotocol.io/liqbot/errors"
)

// DataNode stores state for a Vega Data node.
type DataNode struct {
	hosts       []string // format: host:port
	callTimeout time.Duration
	conn        *grpc.ClientConn
	mu          sync.RWMutex
	wg          sync.WaitGroup
	once        sync.Once
}

// NewDataNode returns a new node.
func NewDataNode(hosts []string, callTimeoutMil int) *DataNode {
	return &DataNode{
		hosts:       hosts,
		callTimeout: time.Duration(callTimeoutMil) * time.Millisecond,
	}
}

// MustDialConnection tries to establish a connection to one of the nodes from a list of locations.
// It is idempotent, while it each call will block the caller until a connection is established.
func (n *DataNode) MustDialConnection(ctx context.Context) {
	n.once.Do(func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		n.wg.Add(len(n.hosts))

		for _, h := range n.hosts {
			go func(host string) {
				defer func() {
					cancel()
					n.wg.Done()
				}()
				n.dialNode(ctx, host)
			}(h)
		}
		n.wg.Wait()
		n.mu.Lock()
		defer n.mu.Unlock()

		if n.conn == nil {
			log.Fatalf("Failed to connect to DataNode")
		}
	})

	n.wg.Wait()
	n.once = sync.Once{}
}

func (n *DataNode) dialNode(ctx context.Context, host string) {
	conn, err := grpc.DialContext(
		ctx,
		host,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		if err != context.Canceled {
			log.Printf("Failed to dial node '%s': %s\n", host, err)
		}
		return
	}

	n.mu.Lock()
	n.conn = conn
	n.mu.Unlock()
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

// ObserveEventBus opens a stream.
func (n *DataNode) ObserveEventBus(ctx context.Context) (client vegaapipb.CoreService_ObserveEventBusClient, err error) {
	msg := "gRPC call failed: ObserveEventBus: %w"
	if n == nil {
		err = fmt.Errorf(msg, e.ErrNil)
		return
	}

	if n.conn == nil || n.conn.GetState() != connectivity.Ready {
		err = fmt.Errorf(msg, e.ErrConnectionNotReady)
		return
	}

	c := vegaapipb.NewCoreServiceClient(n.conn)
	// no timeout on streams
	client, err = c.ObserveEventBus(ctx)
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
