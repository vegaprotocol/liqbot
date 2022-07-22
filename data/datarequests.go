package data

import (
	"errors"
	"fmt"

	"code.vegaprotocol.io/liqbot/types/num"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/protos/vega"
	log "github.com/sirupsen/logrus"
)

func (s *data) getOpenVolume() (int64, error) {
	// Position
	positions, err := s.getPositions()
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

func convertUint256(valueStr string) (value *num.Uint, err error) {
	value, overflowed := num.UintFromString(valueStr, 10)
	if overflowed {
		err = errors.New("invalid uint256, needs to be base 10")
	}
	return
}

func (s *data) getAccount(typ vega.AccountType) (*num.Uint, error) {
	response, err := s.node.PartyAccounts(&dataapipb.PartyAccountsRequest{
		PartyId: s.walletPubKey,
		Asset:   s.settlementAssetID,
		Type:    typ,
	})
	if err != nil {
		return nil, err
	}
	if len(response.Accounts) == 0 {
		s.log.WithFields(log.Fields{
			"type": typ,
		}).Debug("zero accounts for party")
		return num.Zero(), nil
	}
	if len(response.Accounts) > 1 {
		return nil, fmt.Errorf("too many accounts for party: %d", len(response.Accounts))
	}

	return convertUint256(response.Accounts[0].Balance)
}

// getAccountGeneral get this bot's general account balance.
func (s *data) getAccountGeneral() (*num.Uint, error) {
	balance, err := s.getAccount(vega.AccountType_ACCOUNT_TYPE_GENERAL)
	if err != nil {
		return nil, fmt.Errorf("failed to get general account balance: %w", err)
	}

	return balance, nil
}

// getAccountMargin get this bot's margin account balance.
func (s *data) getAccountMargin() (*num.Uint, error) {
	balanceMargin, err := s.getAccount(vega.AccountType_ACCOUNT_TYPE_MARGIN)
	if err != nil {
		return nil, fmt.Errorf("failed to get margin account balance: %w", err)
	}

	return balanceMargin, nil
}

// getAccountBond get this bot's bond account balance.
func (s *data) getAccountBond() (*num.Uint, error) {
	balanceBond, err := s.getAccount(vega.AccountType_ACCOUNT_TYPE_BOND)
	if err != nil {
		return nil, fmt.Errorf("failed to get bond account balance: %w", err)
	}

	return balanceBond, nil
}

// getPositions get this bot's positions.
func (s *data) getPositions() ([]*vega.Position, error) {
	response, err := s.node.PositionsByParty(&dataapipb.PositionsByPartyRequest{
		PartyId:  s.walletPubKey,
		MarketId: s.marketID,
	})
	if err != nil {
		return nil, err
	}

	return response.Positions, nil
}

// getMarketData gets the latest info about the market.
func (s *data) getMarketData() (*vega.MarketData, error) {
	response, err := s.node.MarketDataByID(&dataapipb.MarketDataByIDRequest{MarketId: s.marketID})
	if err != nil {
		return nil, fmt.Errorf("failed to get market data (ID:%s): %w", s.marketID, err)
	}

	return response.MarketData, nil
}
