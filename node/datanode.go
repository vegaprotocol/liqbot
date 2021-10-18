package node

import (
	"context"
	"fmt"
	"net/url"
	"time"

	e "code.vegaprotocol.io/liqbot/errors"
	"code.vegaprotocol.io/liqbot/helpers"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
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

// SubmitTransaction submits a signed v2 transaction
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

// LastBlockHeight gets the latest blockchain height (used for replay protection)
func (n *DataNode) LastBlockHeight() (height uint64, err error) {
	msg := "gRPC call failed: LastBlockHeight: %w"
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
	var response *vegaapipb.LastBlockHeightResponse
	response, err = c.LastBlockHeight(ctx, &vegaapipb.LastBlockHeightRequest{})
	if err != nil {
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	height = response.Height
	return
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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc FeeInfrastructureAccounts(FeeInfrastructureAccountsRequest) returns (FeeInfrastructureAccountsResponse);
// rpc GlobalRewardPoolAccounts(GlobalRewardPoolAccountsRequest) returns (GlobalRewardPoolAccountsResponse);
// rpc Candles(CandlesRequest) returns (CandlesResponse);

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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc MarketsData(MarketsDataRequest) returns (MarketsDataResponse);
// rpc MarketByID(MarketByIDRequest) returns (MarketByIDResponse);
// rpc MarketDepth(MarketDepthRequest) returns (MarketDepthResponse);

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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc OrderByMarketAndID(OrderByMarketAndIDRequest) returns (OrderByMarketAndIDResponse);
// rpc OrderByReference(OrderByReferenceRequest) returns (OrderByReferenceResponse);
// rpc OrdersByMarket(OrdersByMarketRequest) returns (OrdersByMarketResponse);
// rpc OrdersByParty(OrdersByPartyRequest) returns (OrdersByPartyResponse);
// rpc OrderByID(OrderByIDRequest) returns (OrderByIDResponse);
// rpc OrderVersionsByID(OrderVersionsByIDRequest) returns (OrderVersionsByIDResponse);
// rpc MarginLevels(MarginLevelsRequest) returns (MarginLevelsResponse);
// rpc Parties(PartiesRequest) returns (PartiesResponse);
// rpc PartyByID(PartyByIDRequest) returns (PartyByIDResponse);

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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc LastTrade(LastTradeRequest) returns (LastTradeResponse);
// rpc TradesByMarket(TradesByMarketRequest) returns (TradesByMarketResponse);
// rpc TradesByOrder(TradesByOrderRequest) returns (TradesByOrderResponse);
// rpc TradesByParty(TradesByPartyRequest) returns (TradesByPartyResponse);
// rpc GetProposals(GetProposalsRequest) returns (GetProposalsResponse);
// rpc GetProposalsByParty(GetProposalsByPartyRequest) returns (GetProposalsByPartyResponse);
// rpc GetVotesByParty(GetVotesByPartyRequest) returns (GetVotesByPartyResponse);
// rpc GetNewMarketProposals(GetNewMarketProposalsRequest) returns (GetNewMarketProposalsResponse);
// rpc GetUpdateMarketProposals(GetUpdateMarketProposalsRequest) returns (GetUpdateMarketProposalsResponse);
// rpc GetNetworkParametersProposals(GetNetworkParametersProposalsRequest) returns (GetNetworkParametersProposalsResponse);
// rpc GetNewAssetProposals(GetNewAssetProposalsRequest) returns (GetNewAssetProposalsResponse);
// rpc GetProposalByID(GetProposalByIDRequest) returns (GetProposalByIDResponse);
// rpc GetProposalByReference(GetProposalByReferenceRequest) returns (GetProposalByReferenceResponse);
// rpc ObserveGovernance(ObserveGovernanceRequest) returns (stream ObserveGovernanceResponse);
// rpc ObservePartyProposals(ObservePartyProposalsRequest) returns (stream ObservePartyProposalsResponse);
// rpc ObservePartyVotes(ObservePartyVotesRequest) returns (stream ObservePartyVotesResponse);
// rpc ObserveProposalVotes(ObserveProposalVotesRequest) returns (stream ObserveProposalVotesResponse);
// rpc ObserveEventBus(stream ObserveEventBusRequest) returns (stream ObserveEventBusResponse);
// rpc GetNodeData(GetNodeDataRequest) returns (GetNodeDataResponse);
// rpc GetNodes(GetNodesRequest) returns (GetNodesResponse);
// rpc GetNodeByID(GetNodeByIDRequest) returns (GetNodeByIDResponse);
// rpc GetEpoch(GetEpochRequest) returns (GetEpochResponse);
// rpc GetVegaTime(GetVegaTimeRequest) returns (GetVegaTimeResponse);
// rpc AccountsSubscribe(AccountsSubscribeRequest) returns (stream AccountsSubscribeResponse);
// rpc CandlesSubscribe(CandlesSubscribeRequest) returns (stream CandlesSubscribeResponse);
// rpc MarginLevelsSubscribe(MarginLevelsSubscribeRequest) returns (stream MarginLevelsSubscribeResponse);
// rpc MarketDepthSubscribe(MarketDepthSubscribeRequest) returns (stream MarketDepthSubscribeResponse);
// rpc MarketDepthUpdatesSubscribe(MarketDepthUpdatesSubscribeRequest) returns (stream MarketDepthUpdatesSubscribeResponse);
// rpc MarketsDataSubscribe(MarketsDataSubscribeRequest) returns (stream MarketsDataSubscribeResponse);
// rpc OrdersSubscribe(OrdersSubscribeRequest) returns (stream OrdersSubscribeResponse);

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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
		return
	}
	return
}

// rpc TradesSubscribe(TradesSubscribeRequest) returns (stream TradesSubscribeResponse);
// rpc TransferResponsesSubscribe(TransferResponsesSubscribeRequest) returns (stream TransferResponsesSubscribeResponse);
// rpc GetNodeSignaturesAggregate(GetNodeSignaturesAggregateRequest) returns (GetNodeSignaturesAggregateResponse);

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
		err = fmt.Errorf(msg, helpers.ErrorDetail(err))
	}
	return
}

// rpc Assets(AssetsRequest) returns (AssetsResponse);
// rpc EstimateFee(EstimateFeeRequest) returns (EstimateFeeResponse);
// rpc EstimateMargin(EstimateMarginRequest) returns (EstimateMarginResponse);
// rpc ERC20WithdrawalApproval(ERC20WithdrawalApprovalRequest) returns (ERC20WithdrawalApprovalResponse);
// rpc Withdrawal(WithdrawalRequest) returns (WithdrawalResponse);
// rpc Withdrawals(WithdrawalsRequest) returns (WithdrawalsResponse);
// rpc Deposit(DepositRequest) returns (DepositResponse);
// rpc Deposits(DepositsRequest) returns (DepositsResponse);
// rpc NetworkParameters(NetworkParametersRequest) returns (NetworkParametersResponse);
// rpc LiquidityProvisions(LiquidityProvisionsRequest) returns (LiquidityProvisionsResponse);
// rpc OracleSpec(OracleSpecRequest) returns (OracleSpecResponse);
// rpc OracleSpecs(OracleSpecsRequest) returns (OracleSpecsResponse);
// rpc OracleDataBySpec(OracleDataBySpecRequest) returns (OracleDataBySpecResponse);
// rpc GetRewardDetails(GetRewardDetailsRequest) returns (GetRewardDetailsResponse);
// rpc Checkpoints(CheckpointsRequest) returns (CheckpointsResponse);
// rpc Delegations(DelegationsRequest) returns (DelegationsResponse);
// rpc PartyStake(PartyStakeRequest) returns (PartyStakeResponse);
