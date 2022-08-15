package data

import (
	"errors"
	"fmt"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/util"
)

func (d *data) initOpenVolume() error {
	positions, err := d.getPositions()
	if err != nil {
		return fmt.Errorf("failed to get position details: %w", err)
	}

	var openVolume int64
	// If we have not traded yet, then we won't have a position
	if positions != nil {
		if len(positions) != 1 {
			return errors.New("one position item required")
		}
		openVolume = positions[0].OpenVolume
	}

	d.store.MarketSet(types.SetOpenVolume(openVolume))
	return nil
}

// getPositions get this bot's positions.
func (d *data) getPositions() ([]*vega.Position, error) {
	response, err := d.node.PositionsByParty(&dataapipb.PositionsByPartyRequest{
		PartyId:  d.walletPubKey,
		MarketId: d.marketID,
	})
	if err != nil {
		return nil, err
	}

	return response.Positions, nil
}

func (d *data) initBalance() error {
	response, err := d.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: d.walletPubKey,
		Asset:   d.settlementAssetID,
	})
	if err != nil {
		return err
	}

	if len(response.Accounts) == 0 {
		d.log.WithFields(log.Fields{
			"party": d.walletPubKey,
		}).Warning("Party has no accounts")
		return nil
	}

	for _, account := range response.Accounts {
		if err = d.setBalanceByType(account); err != nil {
			d.log.WithFields(
				log.Fields{
					"error":       err.Error(),
					"accountType": account.Type.String(),
				},
			).Error("failed to set account balance")
		}
	}

	return nil
}

func (d *data) setBalanceByType(account *vega.Account) error {
	balance, err := util.ConvertUint256(account.Balance)
	if err != nil {
		return fmt.Errorf("failed to convert account balance: %w", err)
	}

	d.store.BalanceSet(types.SetBalanceByType(account.Type, balance))
	return nil
}

// initMarketData gets the latest info about the market.
func (d *data) initMarketData() error {
	response, err := d.node.MarketDataByID(&dataapipb.MarketDataByIDRequest{MarketId: d.marketID})
	if err != nil {
		return fmt.Errorf("failed to get market data (ID:%s): %w", d.marketID, err)
	}

	md, err := types.FromVegaMD(response.MarketData)
	if err != nil {
		return fmt.Errorf("failed to convert market data: %w", err)
	}

	d.store.MarketSet(types.SetMarketData(md))
	return nil
}
