package data

import (
	"errors"
	"fmt"

	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/util"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	log "github.com/sirupsen/logrus"
)

func (d *data) getOpenVolume() (int64, error) {
	// Position
	positions, err := d.getPositions()
	if err != nil {
		return 0, fmt.Errorf("failed to get position details: %w", err)
	}

	var openVolume int64
	// If we have not traded yet, then we won't have a position
	if positions != nil {
		if len(positions) != 1 {
			return 0, errors.New("one position item required")
		}
		openVolume = positions[0].OpenVolume
	}

	return openVolume, nil
}

func (d *data) getAccount(typ vega.AccountType) (*num.Uint, error) {
	response, err := d.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: d.walletPubKey,
		Asset:   d.settlementAssetID,
		Type:    typ,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Accounts) == 0 {
		d.log.WithFields(log.Fields{
			"type": typ,
		}).Debug("zero accounts for party")
		return num.Zero(), nil
	}
	if len(response.Accounts) > 1 {
		return nil, fmt.Errorf("too many accounts for party: %d", len(response.Accounts))
	}

	return util.ConvertUint256(response.Accounts[0].Balance)
}

// getAccountGeneral get this bot's general account balance.
func (d *data) getAccountGeneral() (*num.Uint, error) {
	balance, err := d.getAccount(vega.AccountType_ACCOUNT_TYPE_GENERAL)
	if err != nil {
		return nil, fmt.Errorf("failed to get general account balance: %w", err)
	}

	return balance, nil
}

// getAccountMargin get this bot's margin account balance.
func (d *data) getAccountMargin() (*num.Uint, error) {
	balanceMargin, err := d.getAccount(vega.AccountType_ACCOUNT_TYPE_MARGIN)
	if err != nil {
		return nil, fmt.Errorf("failed to get margin account balance: %w", err)
	}

	return balanceMargin, nil
}

// getAccountBond get this bot's bond account balance.
func (d *data) getAccountBond() (*num.Uint, error) {
	balanceBond, err := d.getAccount(vega.AccountType_ACCOUNT_TYPE_BOND)
	if err != nil {
		return nil, fmt.Errorf("failed to get bond account balance: %w", err)
	}

	return balanceBond, nil
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

// getMarketData gets the latest info about the market.
func (d *data) getMarketData() (*vega.MarketData, error) {
	response, err := d.node.MarketDataByID(&dataapipb.MarketDataByIDRequest{MarketId: d.marketID})
	if err != nil {
		return nil, fmt.Errorf("failed to get market data (ID:%s): %w", d.marketID, err)
	}

	return response.MarketData, nil
}
