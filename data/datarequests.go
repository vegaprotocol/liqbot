package data

import (
	"errors"
	"fmt"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/types/num"
	"code.vegaprotocol.io/liqbot/util"
)

func (d *data) getOpenVolume() (int64, error) {
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

func (d *data) getAccount() (*types.Balance, error) {
	response, err := d.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: d.walletPubKey,
		Asset:   d.settlementAssetID,
	})
	if err != nil {
		return nil, err
	}

	balance := &types.Balance{
		General: &num.Uint{},
		Margin:  &num.Uint{},
		Bond:    &num.Uint{},
	}

	if len(response.Accounts) == 0 {
		d.log.WithFields(log.Fields{
			"party": d.walletPubKey,
		}).Warning("Party has no accounts")
		return balance, nil
	}

	for _, account := range response.Accounts {
		switch account.Type {
		case vega.AccountType_ACCOUNT_TYPE_GENERAL:
			balance.General, err = util.ConvertUint256(account.Balance)
		case vega.AccountType_ACCOUNT_TYPE_MARGIN:
			balance.Margin, err = util.ConvertUint256(account.Balance)
		case vega.AccountType_ACCOUNT_TYPE_BOND:
			balance.Bond, err = util.ConvertUint256(account.Balance)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to convert account balance: %w", err)
		}
	}

	return balance, nil
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
