package data

import (
	"context"
	"fmt"
	"time"

	"code.vegaprotocol.io/vega/events"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/util"

	"code.vegaprotocol.io/protos/vega"
	coreapipb "code.vegaprotocol.io/protos/vega/api/v1"
	eventspb "code.vegaprotocol.io/protos/vega/events/v1"
	log "github.com/sirupsen/logrus"
)

type data struct {
	log               *log.Entry
	node              DataNode
	walletPubKey      string
	marketID          string
	settlementAssetID string
	store             dataStore
	busEvProc         busEventer
}

func NewDataStream(marketID, pubKey, settlementAssetID string, node DataNode, store dataStore, pauseCh chan types.PauseSignal) (*data, error) {
	d := &data{
		log:       log.WithField("module", "DataStreamer"),
		node:      node,
		store:     store,
		busEvProc: newBusEventProcessor(node, pauseCh),
	}

	d.walletPubKey = pubKey
	d.marketID = marketID
	d.settlementAssetID = settlementAssetID

	go d.store.cache()

	if err := d.setInitialData(); err != nil {
		return nil, fmt.Errorf("failed to set initial data: %w", err)
	}

	d.subscribe()
	return d, nil
}

func (d *data) subscribe() {
	go d.subscribeToMarketEvents()
	go d.subscribeToAccountEvents()
	go d.subscribePositions()
}

// WaitForDepositFinalize is a blocking call that waits for the deposit finalize event to be received
func (d *data) WaitForDepositFinalize(amount *num.Uint) error {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_DEPOSIT,
		},
		PartyId: d.walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			dep := event.GetDeposit()
			// filter out any that are for different assets, or not finalized
			if dep.Asset != d.settlementAssetID || dep.Status != vega.Deposit_STATUS_FINALIZED {
				continue
			}

			if dep.Amount == amount.String() {
				d.log.WithFields(log.Fields{"amount": amount}).Info("Deposit finalized")
				return true, nil
			}
		}
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*40)
	defer cancel()

	d.busEvProc.processEvents(ctx, "DepositData", req, proc)
	return ctx.Err()
}

func (d *data) setInitialData() error {
	// Collateral
	balanceGeneral, err := d.getAccountGeneral()
	if err != nil {
		return fmt.Errorf("failed to get account general: %w", err)
	}

	d.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_GENERAL, balanceGeneral)

	balanceMargin, err := d.getAccountMargin()
	if err != nil {
		return fmt.Errorf("failed to get account margin: %w", err)
	}

	d.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_MARGIN, balanceMargin)

	balanceBond, err := d.getAccountBond()
	if err != nil {
		return fmt.Errorf("failed to get account bond: %w", err)
	}

	d.store.balanceSet(vega.AccountType_ACCOUNT_TYPE_BOND, balanceBond)

	marketData, err := d.getMarketData()
	if err != nil {
		return fmt.Errorf("failed to get market data: %w", err)
	}

	currentPrice, err := util.ConvertUint256(marketData.StaticMidPrice)
	if err != nil {
		return fmt.Errorf("failed to convert current price: %w", err)
	}

	targetStake, err := util.ConvertUint256(marketData.TargetStake)
	if err != nil {
		d.log.WithFields(log.Fields{
			"targetStake": marketData.TargetStake,
		}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
	}

	suppliedStake, err := util.ConvertUint256(marketData.SuppliedStake)
	if err != nil {
		d.log.WithFields(log.Fields{
			"suppliedStake": marketData.SuppliedStake,
		}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
	}

	d.store.marketDataSet(&types.MarketData{
		StaticMidPrice: currentPrice,
		MarkPrice:      &num.Uint{},
		TargetStake:    targetStake,
		SuppliedStake:  suppliedStake,
		TradingMode:    marketData.MarketTradingMode,
	})

	openVolume, err := d.getOpenVolume()
	if err != nil {
		return fmt.Errorf("failed to get open volume: %w", err)
	}

	d.store.openVolumeSet(openVolume)

	return nil
}

func (d *data) subscribeToMarketEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_MARKET_DATA,
		},
		MarketId: d.marketID,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			marketData := event.GetMarketData()

			markPrice, err := util.ConvertUint256(marketData.MarkPrice)
			if err != nil {
				d.log.WithFields(log.Fields{
					"staticMidPrice": marketData.StaticMidPrice,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			staticMidPrice, err := util.ConvertUint256(marketData.StaticMidPrice)
			if err != nil {
				d.log.WithFields(log.Fields{
					"staticMidPrice": marketData.StaticMidPrice,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			targetStake, err := util.ConvertUint256(marketData.TargetStake)
			if err != nil {
				d.log.WithFields(log.Fields{
					"targetStake": marketData.TargetStake,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			suppliedStake, err := util.ConvertUint256(marketData.SuppliedStake)
			if err != nil {
				d.log.WithFields(log.Fields{
					"suppliedStake": marketData.SuppliedStake,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
			}

			d.store.marketDataSet(&types.MarketData{
				StaticMidPrice: staticMidPrice,
				MarkPrice:      markPrice,
				TargetStake:    targetStake,
				SuppliedStake:  suppliedStake,
				TradingMode:    marketData.MarketTradingMode,
			})
		}
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "MarketData", req, proc)
}

// Party related events
func (d *data) subscribeToAccountEvents() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_ACCOUNT,
		},
		PartyId: d.walletPubKey,
	}

	proc := func(rsp *coreapipb.ObserveEventBusResponse) (bool, error) {
		for _, event := range rsp.Events {
			acct := event.GetAccount()
			// filter out any that are for different assets
			if acct.Asset != d.settlementAssetID {
				continue
			}

			bal, err := util.ConvertUint256(acct.Balance)
			if err != nil {
				d.log.WithFields(log.Fields{
					"error":          err,
					"AccountBalance": acct.Balance,
				}).Warning("processEventBusData: failed to unmarshal uint256: error or overflow")
				continue
			}

			d.store.balanceSet(acct.Type, bal)
		}
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "AccountData", req, proc)
}

func (d *data) subscribePositions() {
	req := &coreapipb.ObserveEventBusRequest{
		Type: []eventspb.BusEventType{
			eventspb.BusEventType_BUS_EVENT_TYPE_SETTLE_POSITION,
		},
		PartyId:  d.walletPubKey,
		MarketId: d.marketID,
	}

	proc := func(ev *coreapipb.ObserveEventBusResponse) (bool, error) {
		ctx := context.Background()
		openVolume := d.store.OpenVolume()

		for _, event := range ev.Events {
			posEvt := events.SettlePositionEventFromStream(ctx, event)

			for _, p := range posEvt.Trades() {
				openVolume += p.Size()
			}
		}

		d.store.openVolumeSet(openVolume)
		return false, nil
	}

	d.busEvProc.processEvents(context.Background(), "PositionData", req, proc)
}
