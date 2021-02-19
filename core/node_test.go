package core_test

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"code.vegaprotocol.io/liqbot/core"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
)

type subtest func(t *testing.T, n core.Node)

func runTestOnNodes(t *testing.T, nodes []core.Node, f subtest) {
	for _, n := range nodes {
		f(t, n)
	}
}

func TestNodeNil(t *testing.T) {
	nilNodes := make([]core.Node, 2)

	nilGrpc, err := core.NilGrpcNode()
	assert.NoError(t, err)
	nilNodes[0] = nilGrpc

	nilRest, err := core.NilRestNode()
	assert.NoError(t, err)
	nilNodes[1] = nilRest

	runTestOnNodes(t, nilNodes, testNodeNil)
}

var _ subtest = testNodeNil

func testNodeNil(t *testing.T, n core.Node) {
	name, err := n.GetName()
	assert.Equal(t, core.ErrNil, err)
	assert.Equal(t, "", name)

	addr, err := n.GetAddress()
	assert.Equal(t, core.ErrNil, err)
	assert.Equal(t, url.URL{}, addr)

	mkt, err := n.GetMarket("mkt")
	assert.Equal(t, core.ErrNil, err)
	assert.Equal(t, proto.Market{}, mkt)

	mktDepth, err := n.GetMarketDepth("mkt")
	assert.Equal(t, core.ErrNil, err)
	assert.Equal(t, proto.MarketDepth{}, mktDepth)

	mkts, err := n.GetMarkets()
	assert.Equal(t, core.ErrNil, err)
	assert.Nil(t, mkts)

	tt, err := n.GetTime()
	assert.Equal(t, core.ErrNil, err)
	assert.Equal(t, time.Time{}, tt)

	err = n.UpdateMarkets()
	assert.Equal(t, core.ErrNil, err)

	err = n.UpdateMarketDepths()
	assert.Equal(t, core.ErrNil, err)

	err = n.UpdateTime()
	assert.Equal(t, core.ErrNil, err)
}

func TestGrpcNodeBadAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(100)*time.Millisecond)
	defer cancel()
	nodeAddr := url.URL{Scheme: "grpc", Host: "non.existant.xzxzxz:3002", Path: ""}
	n, err := core.NewGRPCNode(ctx, "grpc_bad", nodeAddr)
	assert.Equal(t, "context deadline exceeded", err.Error())
	assert.Nil(t, n)
}

func TestRestNodeBadAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(100)*time.Millisecond)
	defer cancel()
	nodeAddr := url.URL{Scheme: "http", Host: "non.existant.xzxzxz", Path: "/"}
	n, err := core.NewRESTNode(ctx, "rest_bad", nodeAddr)
	assert.Equal(t, "context deadline exceeded", err.Error())
	assert.Nil(t, n)
}

func TestNode(t *testing.T) {
	nodes := make([]core.Node, 2)

	ctx1, cancel1 := context.WithTimeout(context.Background(), time.Duration(10000)*time.Millisecond)
	defer cancel1()
	grpcNodeName := "grpc_node"
	grpcNodeAddr := url.URL{Scheme: "grpc", Host: "n04.d.vega.xyz:3002", Path: ""}
	grpcNode, err := core.NewGRPCNode(ctx1, grpcNodeName, grpcNodeAddr)
	assert.NoError(t, err)
	if err != nil || grpcNode == nil {
		return
	}
	nodes[0] = grpcNode

	name, err := grpcNode.GetName()
	assert.NoError(t, err)
	assert.Equal(t, grpcNodeName, name)

	addr, err := grpcNode.GetAddress()
	assert.NoError(t, err)
	assert.Equal(t, grpcNodeAddr.String(), addr.String())

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Duration(20000)*time.Millisecond)
	defer cancel2()
	restNodeName := "rest_node"
	restNodeAddr := url.URL{Scheme: "https", Host: "n04.d.vega.xyz", Path: "/"}
	restNode, err := core.NewRESTNode(ctx2, restNodeName, restNodeAddr)
	assert.NoError(t, err)
	if err != nil || restNode == nil {
		return
	}
	nodes[1] = restNode

	name, err = restNode.GetName()
	assert.NoError(t, err)
	assert.Equal(t, restNodeName, name)

	addr, err = restNode.GetAddress()
	assert.NoError(t, err)
	assert.Equal(t, restNodeAddr.String(), addr.String())

	runTestOnNodes(t, nodes, testNode)

	runTestOnNodes(t, nodes, testNodeParallel)
}

var _ subtest = testNode

func testNode(t *testing.T, n core.Node) {
	mkts, err := n.GetMarkets()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(mkts), 3)

	const mktName = "1F0BB6EB5703B099" // ETHBTC

	mkt, err := n.GetMarket(mktName)
	assert.NoError(t, err)
	assert.NotNil(t, mkt)
	mktID := mkt.Id

	mkt, err = n.GetMarket(mktID)
	assert.NoError(t, err)
	assert.NotNil(t, mkt)

	mkt, err = n.GetMarket("fred")
	assert.Equal(t, err, core.ErrMarketNotFound)
	assert.Equal(t, proto.Market{}, mkt)

	mktDepth, err := n.GetMarketDepth(mktID)
	assert.NoError(t, err)
	assert.Equal(t, mktID, mktDepth.MarketId)

	tt, err := n.GetTime()
	assert.NoError(t, err)
	assert.NotNil(t, tt)
}

var _ subtest = testNodeParallel

func testNodeParallel(t *testing.T, n core.Node) {
	log.SetLevel(log.WarnLevel)
	// There is already a core.updateNode(n) running in a goroutine.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100000; i++ {
			_, _ = n.GetTime()
			_, _ = n.GetMarkets()
			_, _ = n.GetMarketDepths()
			_, _ = n.GetMarket("GBPUSD/JAN21")
		}
	}()

	wg.Wait()
}
