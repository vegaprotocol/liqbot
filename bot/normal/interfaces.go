package normal

import (
	"context"
	"net/url"
	"time"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"
	v12 "code.vegaprotocol.io/protos/vega/commands/v1"
	"code.vegaprotocol.io/protos/vega/wallet/v1"

	"code.vegaprotocol.io/liqbot/types"
)

// CoreService implements the gRPC service of the same name.
type CoreService interface {
	SubmitTransaction(req *vegaapipb.SubmitTransactionRequest) (response *vegaapipb.SubmitTransactionResponse, err error)
	// rpc PropagateChainEvent(PropagateChainEventRequest) returns (PropagateChainEventResponse);
	// rpc Statistics(StatisticsRequest) returns (StatisticsResponse);
	LastBlockData() (*vegaapipb.LastBlockHeightResponse, error)
	GetVegaTime() (t time.Time, err error)
	ObserveEventBus() (client vegaapipb.CoreService_ObserveEventBusClient, err error)
}

// CoreStateService implements the gRPC service of the same name.
type CoreStateService interface { // Avoid using this. Use something from DataNode instead
	// rpc ListAccounts(ListAccountsRequest) returns (ListAccountsResponse);
	// ListAssets(req *vegaapipb.ListAssetsRequest) (response *vegaapipb.ListAssetsResponse, err error)
	// rpc ListNetworkParameters(ListNetworkParametersRequest) returns (ListNetworkParametersResponse);
	// rpc ListParties(ListPartiesRequest) returns (ListPartiesResponse);
	// rpc ListValidators(ListValidatorsRequest) returns (ListValidatorsResponse);
	// ListMarkets(req *vegaapipb.ListMarketsRequest) (response *vegaapipb.ListMarketsResponse, err error)
	// rpc ListProposals(ListProposalsRequest) returns (ListProposalsResponse);
	// rpc ListMarketsData(ListMarketsDataRequest) returns (ListMarketsDataResponse);
	// rpc ListVotes(ListVotesRequest) returns (ListVotesResponse);
	// rpc ListPartiesStake(ListPartiesStakeRequest) returns (ListPartiesStakeResponse);
	// rpc ListDelegations(ListDelegationsRequest) returns (ListDelegationsResponse);
}

// TradingDataService implements the gRPC service of the same name.
type TradingDataService interface {
	// rpc MarketAccounts(MarketAccountsRequest) returns (MarketAccountsResponse);
	// rpc PartyAccounts(PartyAccountsRequest) returns (PartyAccountsResponse);
	PartyAccounts(req *dataapipb.PartyAccountsRequest) (response *dataapipb.PartyAccountsResponse, err error)
	// rpc FeeInfrastructureAccounts(FeeInfrastructureAccountsRequest) returns (FeeInfrastructureAccountsResponse);
	// rpc GlobalRewardPoolAccounts(GlobalRewardPoolAccountsRequest) returns (GlobalRewardPoolAccountsResponse);
	// rpc Candles(CandlesRequest) returns (CandlesResponse);
	MarketDataByID(req *dataapipb.MarketDataByIDRequest) (response *dataapipb.MarketDataByIDResponse, err error)
	// rpc MarketsData(MarketsDataRequest) returns (MarketsDataResponse);
	// rpc MarketByID(MarketByIDRequest) returns (MarketByIDResponse);
	// rpc MarketDepth(MarketDepthRequest) returns (MarketDepthResponse);
	Markets(req *dataapipb.MarketsRequest) (response *dataapipb.MarketsResponse, err error)
	// rpc OrderByMarketAndID(OrderByMarketAndIDRequest) returns (OrderByMarketAndIDResponse);
	// rpc OrderByReference(OrderByReferenceRequest) returns (OrderByReferenceResponse);
	// rpc OrdersByMarket(OrdersByMarketRequest) returns (OrdersByMarketResponse);
	// rpc OrdersByParty(OrdersByPartyRequest) returns (OrdersByPartyResponse);
	// rpc OrderByID(OrderByIDRequest) returns (OrderByIDResponse);
	// rpc OrderVersionsByID(OrderVersionsByIDRequest) returns (OrderVersionsByIDResponse);
	// rpc MarginLevels(MarginLevelsRequest) returns (MarginLevelsResponse);
	// rpc Parties(PartiesRequest) returns (PartiesResponse);
	// rpc PartyByID(PartyByIDRequest) returns (PartyByIDResponse);
	PositionsByParty(req *dataapipb.PositionsByPartyRequest) (response *dataapipb.PositionsByPartyResponse, err error)
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
	PositionsSubscribe(req *dataapipb.PositionsSubscribeRequest) (client dataapipb.TradingDataService_PositionsSubscribeClient, err error)
	// rpc TradesSubscribe(TradesSubscribeRequest) returns (stream TradesSubscribeResponse);
	// rpc TransferResponsesSubscribe(TransferResponsesSubscribeRequest) returns (stream TransferResponsesSubscribeResponse);
	// rpc GetNodeSignaturesAggregate(GetNodeSignaturesAggregateRequest) returns (GetNodeSignaturesAggregateResponse);
	AssetByID(req *dataapipb.AssetByIDRequest) (response *dataapipb.AssetByIDResponse, err error)
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
}

// CoreNode is a Vega Core node
// type CoreNode interface {
// 	GetAddress() (url.URL, error)
// 	CoreService
// 	CoreStateService
// }

// DataNode is a Vega Data node
//go:generate go run github.com/golang/mock/mockgen -destination mocks/datanode_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal DataNode
type DataNode interface {
	GetAddress() (url.URL, error)
	CoreService
	TradingDataService
}

// PricingEngine is the source of price information from the price proxy.
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/bot/normal PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

type WalletClient interface {
	CreateWallet(ctx context.Context, name, passphrase string) error
	LoginWallet(ctx context.Context, name, passphrase string) error
	ListPublicKeys(ctx context.Context) ([]string, error)
	GenerateKeyPair(ctx context.Context, passphrase string, meta []types.Meta) (*types.Key, error)
	SignTx(ctx context.Context, req *v1.SubmitTransactionRequest) (*v12.Transaction, error)
}
