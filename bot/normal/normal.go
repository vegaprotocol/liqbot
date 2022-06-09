package normal

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/types/num"

	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	vegaapipb "code.vegaprotocol.io/protos/vega/api/v1"
	commandspb "code.vegaprotocol.io/protos/vega/commands/v1"
	walletpb "code.vegaprotocol.io/protos/vega/wallet/v1"
	vgcrypto "code.vegaprotocol.io/shared/libs/crypto"
	"code.vegaprotocol.io/vegawallet/wallet"
	"code.vegaprotocol.io/vegawallet/wallets"
	log "github.com/sirupsen/logrus"
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

// Bot represents one Normal liquidity bot.
type Bot struct {
	config                 config.BotConfig
	active                 bool
	log                    *log.Entry
	pricingEngine          PricingEngine
	settlementAssetID      string
	settlementAssetAddress string
	stopPosMgmt            chan bool
	stopPriceSteer         chan bool
	strategy               *Strategy
	market                 *vega.Market
	node                   DataNode

	balanceGeneral *num.Uint
	balanceMargin  *num.Uint
	balanceBond    *num.Uint

	walletServer     *wallets.Handler
	walletPassphrase string
	walletPubKey     string // "58595a" ...

	buyShape   []*vega.LiquidityOrder
	sellShape  []*vega.LiquidityOrder
	marketData *vega.MarketData

	currentPrice       *num.Uint
	staticMidPrice     *num.Uint
	openVolume         int64
	previousOpenVolume int64

	// These flags are used for the streaming systems to let
	// the app know if they are up and working
	eventStreamLive    bool
	positionStreamLive bool

	// Flag to indicate if we have already placed auction orders
	auctionOrdersPlaced bool
}

// New returns a new instance of Bot.
func New(config config.BotConfig, pe PricingEngine, ws *wallets.Handler) (b *Bot, err error) {
	b = &Bot{
		config: config,
		log: log.WithFields(log.Fields{
			"bot":  config.Name,
			"node": config.Location,
		}),
		pricingEngine:       pe,
		walletServer:        ws,
		eventStreamLive:     false,
		positionStreamLive:  false,
		auctionOrdersPlaced: false,
	}

	b.strategy, err = validateStrategyConfig(config.StrategyDetails)
	if err != nil {
		err = fmt.Errorf("failed to read strategy details: %w", err)
		return
	}
	b.log.WithFields(log.Fields{
		"strategy": b.strategy.String(),
	}).Debug("read strategy config")

	return
}

// Start starts the liquidity bot goroutine(s).
func (b *Bot) Start() error {
	_, err := b.setupWallet()
	if err != nil {
		return fmt.Errorf("failed to setup wallet: %w", err)
	}

	b.node, err = node.NewDataNode(
		url.URL{Host: b.config.Location},
		time.Duration(b.config.ConnectTimeout)*time.Millisecond,
		time.Duration(b.config.CallTimeout)*time.Millisecond,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to Vega gRPC node: %w", err)
	}
	b.log.WithFields(log.Fields{
		"address": b.config.Location,
	}).Debug("Connected to Vega gRPC node")

	marketsResponse, err := b.node.Markets(&dataapipb.MarketsRequest{})
	if err != nil {
		return fmt.Errorf("failed to get markets: %w", err)
	}
	b.market = nil
	for _, mkt := range marketsResponse.Markets {
		instrument := mkt.TradableInstrument.GetInstrument()
		if instrument != nil {
			md := instrument.Metadata
			base := ""
			quote := ""
			for _, tag := range md.Tags {
				parts := strings.Split(tag, ":")
				if len(parts) == 2 {
					if parts[0] == "quote" {
						quote = parts[1]
					}
					if parts[0] == "base" || parts[0] == "ticker" {
						base = parts[1]
					}
				}
			}
			if base == b.config.InstrumentBase && quote == b.config.InstrumentQuote {
				future := mkt.TradableInstrument.Instrument.GetFuture()
				if future != nil {
					b.settlementAssetID = future.SettlementAsset
					b.market = mkt
					break
				}
			}
		}
	}
	if b.market == nil {
		return fmt.Errorf("failed to find futures markets: base/ticker=%s, quote=%s", b.config.InstrumentBase, b.config.InstrumentQuote)
	}
	b.log.WithFields(log.Fields{
		"id":                b.market.Id,
		"base/ticker":       b.config.InstrumentBase,
		"quote":             b.config.InstrumentQuote,
		"settlementAssetID": b.settlementAssetID,
	}).Info("Fetched market info")

	// Use the settlementAssetID to lookup the settlement ethereum address
	assetResponse, err := b.node.AssetByID(&dataapipb.AssetByIDRequest{Id: b.settlementAssetID})
	if err != nil {
		return fmt.Errorf("unable to look up asset details for %s", b.settlementAssetID)
	}
	erc20 := assetResponse.Asset.Details.GetErc20()
	if erc20 != nil {
		b.settlementAssetAddress = erc20.ContractAddress
	} else {
		b.settlementAssetAddress = ""
	}

	b.balanceGeneral = num.Zero()
	b.balanceMargin = num.Zero()

	b.active = true
	b.stopPosMgmt = make(chan bool)
	b.stopPriceSteer = make(chan bool)

	err = b.initialiseData()
	if err != nil {
		return fmt.Errorf("failed to initialise data: %w", err)
	}
	go b.runPositionManagement()
	go b.runPriceSteering()

	return nil
}

// Stop stops the liquidity bot goroutine(s).
func (b *Bot) Stop() {
	if !b.active {
		return
	}

	b.stopPosMgmt <- true
	close(b.stopPosMgmt)
	b.stopPriceSteer <- true
	close(b.stopPriceSteer)
}

// GetTraderDetails returns information relating to the trader.
func (b *Bot) GetTraderDetails() string {
	name := b.config.Name
	pubKey := b.walletPubKey
	settlementVegaAssetID := b.settlementAssetID
	settlementEthereumContractAddress := b.settlementAssetAddress

	return "{\"name\":\"" + name + "\",\"pubKey\":\"" + pubKey + "\",\"settlementVegaAssetID\":\"" +
		settlementVegaAssetID + "\",\"settlementEthereumContractAddress\":\"" +
		settlementEthereumContractAddress + "\"}"
}

func (b *Bot) canPlaceOrders() bool {
	return b.marketData.MarketTradingMode == vega.Market_TRADING_MODE_CONTINUOUS
}

func mulFrac(n *num.Uint, x float64, precision float64) *num.Uint {
	val := num.NewUint(uint64(x * math.Pow(10, precision)))
	val.Mul(val, n)
	val.Div(val, num.NewUint(uint64(math.Pow(10, precision))))
	return val
}

func (b *Bot) sendLiquidityProvision(buys, sells []*vega.LiquidityOrder) error {
	// CommitmentAmount is the fractional commitment value * total collateral
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	commitment := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionSubmission{
		LiquidityProvisionSubmission: &commandspb.LiquidityProvisionSubmission{
			Fee:              b.config.StrategyDetails.Fee,
			MarketId:         b.market.Id,
			CommitmentAmount: commitment.String(),
			Buys:             buys,
			Sells:            sells,
		},
	}
	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}
	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionSubmission(%v): %w", cmd, err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionSubmission")
	return nil
}

func (b *Bot) sendLiquidityProvisionAmendment(buys, sells []*vega.LiquidityOrder) error {
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	commitment := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)

	if commitment == num.NewUint(0) {
		return b.sendLiquidityProvisionCancellation(balTotal)
	}

	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionAmendment{
		LiquidityProvisionAmendment: &commandspb.LiquidityProvisionAmendment{
			MarketId:         b.market.Id,
			CommitmentAmount: commitment.String(),
			Fee:              b.config.StrategyDetails.Fee,
			Sells:            sells,
			Buys:             buys,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionAmendment: %w", err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         commitment,
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *Bot) sendLiquidityProvisionCancellation(balTotal *num.Uint) error {
	cmd := &walletpb.SubmitTransactionRequest_LiquidityProvisionCancellation{
		LiquidityProvisionCancellation: &commandspb.LiquidityProvisionCancellation{
			MarketId: b.market.Id,
		},
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}

	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit LiquidityProvisionCancellation: %w", err)
	}
	b.log.WithFields(log.Fields{
		"commitment":         num.NewUint(0),
		"commitmentFraction": b.strategy.CommitmentFraction,
		"balanceTotal":       balTotal,
	}).Debug("Submitted LiquidityProvisionAmendment")

	return nil
}

func (b *Bot) checkForShapeChange() {
	var shape string
	if b.openVolume <= 0 {
		shape = "longening"
		b.buyShape = b.strategy.LongeningShape.Buys
		b.sellShape = b.strategy.LongeningShape.Sells
	} else {
		shape = "shortening"
		b.buyShape = b.strategy.ShorteningShape.Buys
		b.sellShape = b.strategy.ShorteningShape.Sells
	}

	b.log.WithFields(log.Fields{
		"currentPrice":   b.currentPrice,
		"balanceGeneral": b.balanceGeneral,
		"balanceMargin":  b.balanceMargin,
		"openVolume":     b.openVolume,
		"shape":          shape,
	}).Debug("Position management info")

	// If we flipped then send the new LP order
	if (b.openVolume > 0 && b.previousOpenVolume <= 0) ||
		(b.openVolume < 0 && b.previousOpenVolume >= 0) {
		b.log.WithFields(log.Fields{"shape": shape}).Debug("Flipping LP direction")
		err := b.sendLiquidityProvisionAmendment(b.buyShape, b.sellShape)
		if err != nil {
			b.log.WithFields(log.Fields{
				"error": err.Error(),
			}).Warning("Failed to send liquidity provision")
		} else {
			b.previousOpenVolume = b.openVolume
		}
	}
}

func (b *Bot) checkPositionManagement() {
	if !b.canPlaceOrders() {
		// Only allow position management during continuous trading
		return
	}
	var shouldPlaceOrder bool

	// TODO: add calculations
	// posMarginCost := calculatePositionMarginCost(b.openVolume, b.currentPrice, nil)
	// maxCost := uint64((1.0-b.strategy.StakeFraction-b.strategy.OrdersFraction)*b.balanceGeneral.Float64()

	// if posMarginCost > maxCost {
	// 	if b.openVolume > 0 {
	// 		shouldSell = true
	// 	} else if b.openVolume < 0 {
	// 		shouldBuy = true
	// 	}
	// }
	var size uint64
	var side vega.Side
	if b.openVolume >= 0 && num.NewUint(uint64(b.openVolume)).GT(b.strategy.MaxLong.Get()) {
		shouldPlaceOrder = true
		size = mulFrac(num.NewUint(uint64(b.openVolume)), b.strategy.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_SELL
	} else if b.openVolume < 0 && num.NewUint(uint64(-b.openVolume)).GT(b.strategy.MaxShort.Get()) {
		shouldPlaceOrder = true
		size = mulFrac(num.NewUint(uint64(-b.openVolume)), b.strategy.PosManagementFraction, 15).Uint64()
		side = vega.Side_SIDE_BUY
	}

	if !shouldPlaceOrder {
		return
	}

	err := b.submitOrder(size, num.Zero(), side, vega.Order_TIME_IN_FORCE_IOC, vega.Order_TYPE_MARKET, "PosManagement", 0)
	if err != nil {
		b.log.WithFields(log.Fields{
			"error": err,
			"side":  side,
			"size":  size,
		}).Warning("Failed to place a position management order")
	}
}

func (b *Bot) signSubmitTx(
	submitTxReq *walletpb.SubmitTransactionRequest,
	blockData *vegaapipb.LastBlockHeightResponse,
) error {
	msg := "failed to sign+submit tx: %w"
	if blockData == nil {
		var err error
		blockData, err = b.node.LastBlockData()
		if err != nil {
			return fmt.Errorf(msg, fmt.Errorf("failed to get block height: %w", err))
		}
	}

	signedTx, err := b.walletServer.SignTx(b.config.Name, submitTxReq, blockData.Height)
	if err != nil {
		return fmt.Errorf(msg, fmt.Errorf("failed to sign tx: %w", err))
	}

	tid := vgcrypto.RandomHash()
	powNonce, _, err := vgcrypto.PoW(blockData.Hash, tid, uint(blockData.SpamPowDifficulty), vgcrypto.Sha3)
	if err != nil {
		return fmt.Errorf(msg, fmt.Errorf("failed to generate proof of work: %w", err))
	}
	signedTx.Pow = &commandspb.ProofOfWork{
		Tid:   tid,
		Nonce: powNonce,
	}

	submitReq := &vegaapipb.SubmitTransactionRequest{
		Tx:   signedTx,
		Type: vegaapipb.SubmitTransactionRequest_TYPE_ASYNC,
	}
	submitResponse, err := b.node.SubmitTransaction(submitReq)
	if err != nil {
		return fmt.Errorf(msg, fmt.Errorf("failed to submit signed tx: %w", err))
	}
	if !submitResponse.Success {
		return fmt.Errorf(msg, errors.New("success=false"))
	}
	return nil
}

func (b *Bot) submitOrder(
	size uint64,
	price *num.Uint,
	side vega.Side,
	tif vega.Order_TimeInForce,
	orderType vega.Order_Type,
	reference string,
	secondsFromNow int64,
) error {
	cmd := &walletpb.SubmitTransactionRequest_OrderSubmission{
		OrderSubmission: &commandspb.OrderSubmission{
			MarketId:    b.market.Id,
			Price:       "", // added below
			Size:        size,
			Side:        side,
			TimeInForce: tif,
			ExpiresAt:   0, // added below
			Type:        orderType,
			Reference:   reference,
			PeggedOrder: nil,
		},
	}
	if tif == vega.Order_TIME_IN_FORCE_GTT {
		cmd.OrderSubmission.ExpiresAt = time.Now().UnixNano() + (secondsFromNow * 1000000000)
	}
	if orderType != vega.Order_TYPE_MARKET {
		cmd.OrderSubmission.Price = price.String()
	}

	submitTxReq := &walletpb.SubmitTransactionRequest{
		PubKey:  b.walletPubKey,
		Command: cmd,
	}
	err := b.signSubmitTx(submitTxReq, nil)
	if err != nil {
		return fmt.Errorf("failed to submit OrderSubmission: %w", err)
	}
	return nil
}

func (b *Bot) checkInitialMargin() error {
	// Turn the shapes into a set of orders scaled by commitment
	balTotal := num.Sum(b.balanceGeneral, b.balanceMargin, b.balanceBond)
	obligation := mulFrac(balTotal, b.strategy.CommitmentFraction, 15)
	buyOrders := b.calculateOrderSizes(b.market.Id, b.walletPubKey, obligation, b.buyShape)
	sellOrders := b.calculateOrderSizes(b.market.Id, b.walletPubKey, obligation, b.sellShape)

	buyRisk := float64(0.01)
	sellRisk := float64(0.01)

	buyCost := b.calculateMarginCost(buyRisk, buyOrders)
	sellCost := b.calculateMarginCost(sellRisk, sellOrders)

	shapeMarginCost := num.Max(buyCost, sellCost)

	avail := mulFrac(b.balanceGeneral, b.strategy.OrdersFraction, 15)

	if avail.LT(shapeMarginCost) {
		var missingPercent string
		if avail.EQUint64(0) {
			missingPercent = "Inf"
		} else {
			x := num.UintChain(shapeMarginCost).Sub(avail).Mul(num.NewUint(100)).Div(avail).Get()
			missingPercent = fmt.Sprintf("%v%%", x)
		}
		b.log.WithFields(log.Fields{
			"available":      avail,
			"cost":           shapeMarginCost,
			"missing":        num.Zero().Sub(avail, shapeMarginCost),
			"missingPercent": missingPercent,
		}).Error("Not enough collateral to safely keep orders up given current price, risk parameters and supplied default shapes.")
		return errors.New("not enough collateral")
	}
	return nil
}

func (b *Bot) initialiseData() error {
	var err error

	err = b.lookupInitialValues()
	if err != nil {
		b.log.WithFields(log.Fields{"error": err.Error()}).
			Debug("Stopping position management as we could not get initial values")
		return err
	}

	if !b.eventStreamLive {
		err = b.subscribeToEvents()
		if err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).
				Debug("Unable to subscribe to event bus feeds")
			return err
		}
	}

	if !b.positionStreamLive {
		err = b.subscribePositions()
		if err != nil {
			b.log.WithFields(log.Fields{"error": err.Error()}).
				Debug("Unable to subscribe to event bus feeds")
			return err
		}
	}
	return nil
}

// Divide the auction amount into 10 orders and place them randomly
// around the current price at upto 50+/- from it.
func (b *Bot) placeAuctionOrders() {
	// Check we have not placed them already
	if b.auctionOrdersPlaced {
		return
	}
	// Check we have a currentPrice we can use
	if b.currentPrice.EQUint64(0) {
		return
	}

	// Place the random orders split into
	totalVolume := num.Zero()
	rand.Seed(time.Now().UnixNano())
	for totalVolume.LT(b.config.StrategyDetails.AuctionVolume.Get()) {
		remaining := num.Zero().Sub(b.config.StrategyDetails.AuctionVolume.Get(), totalVolume)
		size := num.Min(num.UintChain(b.config.StrategyDetails.AuctionVolume.Get()).Div(num.NewUint(10)).Add(num.NewUint(1)).Get(), remaining)
		// #nosec G404
		price := num.Zero().Add(b.currentPrice, num.NewUint(uint64(rand.Int63n(100)-50)))
		side := vega.Side_SIDE_BUY
		// #nosec G404
		if rand.Intn(2) == 0 {
			side = vega.Side_SIDE_SELL
		}
		err := b.submitOrder(size.Uint64(), price, side, vega.Order_TIME_IN_FORCE_GTT, vega.Order_TYPE_LIMIT, "AuctionOrder", 330)
		if err == nil {
			totalVolume = num.Zero().Add(totalVolume, size)
		} else {
			// We failed to send an order so stop trying to send anymore
			break
		}
	}
	b.auctionOrdersPlaced = true
}

func (b *Bot) runPositionManagement() {
	var err error
	var firstTime bool = true

	// We always start off with longening shapes
	b.buyShape = b.strategy.LongeningShape.Buys
	b.sellShape = b.strategy.LongeningShape.Sells

	sleepTime := b.strategy.PosManagementSleepMilliseconds
	for {
		select {
		case <-b.stopPosMgmt:
			b.log.Debug("Stopping bot position management")
			b.active = false
			return

		default:
			// At the start of each loop, wait for positive general account balance. This is in case the network has
			// been restarted.
			if firstTime {
				err = b.checkInitialMargin()
				if err != nil {
					b.active = false
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Error("Failed initial margin check")
					return
				}
				// Submit LP order to market.
				err = b.sendLiquidityProvision(b.buyShape, b.sellShape)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Error("Failed to send liquidity provision order")
				}
				firstTime = false
			}

			// Only update liquidity and position if we are not in auction
			if b.canPlaceOrders() {
				b.auctionOrdersPlaced = false
				b.checkForShapeChange()
				b.checkPositionManagement()
			} else {
				b.placeAuctionOrders()
			}

			// If we have lost the incoming streams we should attempt to reconnect
			for !b.positionStreamLive || !b.eventStreamLive {
				b.log.WithFields(log.Fields{
					"eventStreamLive":    b.eventStreamLive,
					"positionStreamLive": b.positionStreamLive,
				}).Debug("Stream info")
				err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
				if err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).
						Debug("Stopping bot position management")
					b.active = false
					return
				}

				err = b.initialiseData()
				if err != nil {
					b.log.WithFields(log.Fields{"error": err.Error()}).Debug("Failed to initialise data")
					continue
				}
			}

			err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPosMgmt)
			if err != nil {
				b.log.WithFields(log.Fields{"error": err.Error()}).
					Debug("Stopping bot position management")
				b.active = false
				return
			}
		}
	}
}

// calculateOrderSizes calculates the size of the orders using the total commitment, price, distance from mid and chance
// of trading liquidity.supplied.updateSizes(obligation, currentPrice, liquidityOrders, true, minPrice, maxPrice).
func (b *Bot) calculateOrderSizes(marketID, partyID string, obligation *num.Uint, liquidityOrders []*vega.LiquidityOrder) []*vega.Order {
	orders := make([]*vega.Order, 0, len(liquidityOrders))
	// Work out the total proportion for the shape
	totalProportion := num.Zero()
	for _, order := range liquidityOrders {
		totalProportion.Add(totalProportion, num.NewUint(uint64(order.Proportion)))
	}

	// Now size up the orders and create the real order objects
	for _, lo := range liquidityOrders {
		size := num.UintChain(obligation).Mul(num.NewUint(uint64(lo.Proportion))).Mul(num.NewUint(10)).Div(totalProportion).Div(b.currentPrice).Get()
		peggedOrder := vega.PeggedOrder{
			Reference: lo.Reference,
			Offset:    lo.Offset,
		}

		order := vega.Order{
			MarketId:    marketID,
			PartyId:     partyID,
			Side:        vega.Side_SIDE_BUY,
			Remaining:   size.Uint64(),
			Size:        size.Uint64(),
			TimeInForce: vega.Order_TIME_IN_FORCE_GTC,
			Type:        vega.Order_TYPE_LIMIT,
			PeggedOrder: &peggedOrder,
		}
		orders = append(orders, &order)
	}
	return orders
}

// calculateMarginCost estimates the margin cost of the set of orders.
func (b *Bot) calculateMarginCost(risk float64, orders []*vega.Order) *num.Uint {
	// totalMargin := num.Zero()
	margins := make([]*num.Uint, len(orders))
	for i, order := range orders {
		if order.Side == vega.Side_SIDE_BUY {
			margins[i] = num.NewUint(1 + order.Size)
		} else {
			margins[i] = num.NewUint(order.Size)
		}
	}
	totalMargin := num.UintChain(num.NewUint(0)).Add(margins...).Mul(b.currentPrice).Get()
	totalMargin = mulFrac(totalMargin, risk, 15)
	return totalMargin
}

func (b *Bot) runPriceSteering() {
	var externalPrice *num.Uint
	var currentPrice *num.Uint
	var err error
	var externalPriceResponse ppservice.PriceResponse

	ppcfg := ppconfig.PriceConfig{
		Base:   b.config.InstrumentBase,
		Quote:  b.config.InstrumentQuote,
		Wander: true,
	}

	sleepTime := 1000.0 / b.strategy.MarketPriceSteeringRatePerSecond
	for {
		select {
		case <-b.stopPriceSteer:
			b.log.Debug("Stopping bot market price steering")
			b.active = false
			return

		default:
			canPlaceOrders := b.canPlaceOrders()
			if b.strategy.PriceSteerOrderScale > 0 && canPlaceOrders {
				externalPriceResponse, err = b.pricingEngine.GetPrice(ppcfg)
				if err != nil {
					b.log.WithFields(log.Fields{
						"error": err.Error(),
					}).Warning("Failed to get external price")
					externalPrice = num.Zero()
					currentPrice = num.Zero()
				} else {
					externalPrice = num.UintChain(num.NewUint(uint64(externalPriceResponse.Price))).Mul(num.NewUint(uint64(math.Pow10(int(b.market.DecimalPlaces))))).Get()
					currentPrice = b.staticMidPrice
				}

				if err == nil && currentPrice != nil && externalPrice != nil && !externalPrice.IsZero() {
					shouldMove := "no"
					// We only want to steer the price if the external and market price
					// are greater than a certain percentage apart
					var currentDiff *num.Uint

					// currentDiff = 100 * Abs(currentPrice - externalPrice) / externalPrice
					if currentPrice.GT(externalPrice) {
						currentDiff = num.Zero().Sub(currentPrice, externalPrice)
					} else {
						currentDiff = num.Zero().Sub(externalPrice, currentPrice)
					}
					currentDiff = num.UintChain(currentDiff).Mul(num.NewUint(100)).Div(externalPrice).Get()
					if currentDiff.GT(num.NewUint(uint64(100.0 * b.strategy.MinPriceSteerFraction))) {
						var side vega.Side
						if externalPrice.GT(currentPrice) {
							side = vega.Side_SIDE_BUY
							shouldMove = "UP"
						} else {
							side = vega.Side_SIDE_SELL
							shouldMove = "DN"
						}

						// Now we call into the maths heavy function to find out
						// what price and size of the order we should place
						price, size, priceError := b.GetRealisticOrderDetails(externalPrice)

						if priceError != nil {
							b.log.WithFields(log.Fields{"error": priceError.Error()}).
								Fatal("Unable to get realistic order details for price steering")
						}

						size = mulFrac(size, b.strategy.PriceSteerOrderScale, 15)
						b.log.WithFields(log.Fields{
							"size":  size,
							"side":  side,
							"price": price,
						}).Debug("Submitting order")

						err = b.submitOrder(
							size.Uint64(),
							price,
							side,
							vega.Order_TIME_IN_FORCE_GTT,
							vega.Order_TYPE_LIMIT,
							"PriceSteeringOrder",
							int64(b.strategy.LimitOrderDistributionParams.GttLength))
					}
					b.log.WithFields(log.Fields{
						"currentPrice":  currentPrice,
						"externalPrice": externalPrice,
						"diff":          num.Zero().Sub(externalPrice, b.currentPrice),
						"shouldMove":    shouldMove,
					}).Debug("Steering info")
				}

				if err == nil {
					sleepTime = 1000.0 / b.strategy.MarketPriceSteeringRatePerSecond
				} else {
					if sleepTime < 29000 {
						sleepTime += 1000
					}
					b.log.WithFields(log.Fields{
						"error":     err.Error(),
						"sleepTime": sleepTime,
					}).Warning("Error during price steering")
				}
			} else {
				b.log.WithFields(log.Fields{
					"PriceSteerOrderScale": b.strategy.PriceSteerOrderScale,
					"canPlaceOrders":       canPlaceOrders,
				}).Debug("Price steering: Cannot place orders")
			}
			err = doze(time.Duration(sleepTime)*time.Millisecond, b.stopPriceSteer)
			if err != nil {
				b.log.Debug("Stopping bot market price steering")
				b.active = false
				return
			}
		}
	}
}

// GetRealisticOrderDetails uses magic to return a realistic order price and size.
func (b *Bot) GetRealisticOrderDetails(externalPrice *num.Uint) (price, size *num.Uint, err error) {
	// Collect stuff from config that's common to all methods
	method := b.strategy.LimitOrderDistributionParams.Method

	sigma := b.strategy.TargetLNVol
	tgtTimeHorizonHours := b.strategy.LimitOrderDistributionParams.TgtTimeHorizonHours
	tgtTimeHorizonYrFrac := tgtTimeHorizonHours / 24.0 / 365.25
	numOrdersPerSec := b.strategy.MarketPriceSteeringRatePerSecond
	N := 3600 * numOrdersPerSec / tgtTimeHorizonHours
	tickSize := 1.0 / math.Pow(10, float64(b.market.DecimalPlaces))
	delta := float64(b.strategy.LimitOrderDistributionParams.NumTicksFromMid) * tickSize

	// this converts something like BTCUSD 3912312345 (five decimal places)
	// to 39123.12345 float.
	M0 := num.Zero().Div(externalPrice, num.NewUint(uint64(math.Pow(10, float64(b.market.DecimalPlaces)))))

	var priceFloat float64
	size = num.NewUint(1)
	switch method {
	case DiscreteThreeLevel:
		priceFloat, err = GeneratePriceUsingDiscreteThreeLevel(M0.Float64(), delta, sigma, tgtTimeHorizonYrFrac, N)

		// we need to add back decimals
		price = num.NewUint(uint64(math.Round(priceFloat * math.Pow(10, float64(b.market.DecimalPlaces)))))
		return
	case CoinAndBinomial:
		price = externalPrice
		return
	default:
		err = fmt.Errorf("Method for generating price distributions not recognised: %w", err)
		return
	}
}

func (b *Bot) setupWallet() (mnemonic string, err error) {
	b.walletPassphrase = "123"

	err = b.walletServer.LoginWallet(b.config.Name, b.walletPassphrase)
	if err != nil {
		if err == wallets.ErrWalletDoesNotExists {
			mnemonic, err = b.walletServer.CreateWallet(b.config.Name, b.walletPassphrase)
			if err != nil {
				return "", fmt.Errorf("failed to create wallet: %w", err)
			}
			b.log.Debug("Created and logged into wallet")
		} else {
			return "", fmt.Errorf("failed to log in to wallet: %w", err)
		}
	} else {
		b.log.Debug("Logged into wallet")
	}

	if b.walletPubKey == "" {
		var keys []wallet.PublicKey
		keys, err = b.walletServer.ListPublicKeys(b.config.Name)
		if err != nil {
			return "", fmt.Errorf("failed to list public keys: %w", err)
		}
		if len(keys) == 0 {
			var key wallet.KeyPair
			key, err = b.walletServer.GenerateKeyPair(b.config.Name, b.walletPassphrase, []wallet.Meta{})
			if err != nil {
				return "", fmt.Errorf("failed to generate keypair: %w", err)
			}
			b.walletPubKey = key.PublicKey()
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Created keypair")
		} else {
			b.walletPubKey = keys[0].Key()
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Using existing keypair")
		}
	}
	b.log = b.log.WithFields(log.Fields{"pubkey": b.walletPubKey})
	return mnemonic, err
}
