package normal

import (
	"errors"
	"fmt"

	"code.vegaprotocol.io/liqbot/types/num"

	dataapipbv2 "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
	log "github.com/sirupsen/logrus"
)

// As the streaming service only gives us data when it changes
// we need to look up the initial data manually.
func (b *Bot) lookupInitialValues() error {
	// Collateral
	err := b.getAccountGeneral()
	if err != nil {
		return fmt.Errorf("Failed to get general account details: %w", err)
	}
	err = b.getAccountMargin()
	if err != nil {
		return fmt.Errorf("Failed to get margin account details: %w", err)
	}
	err = b.getAccountBond()
	if err != nil {
		return fmt.Errorf("Failed to get bond account details: %w", err)
	}

	// Market data (currentPrice, auction etc)
	err = b.getMarketData()
	if err != nil {
		return fmt.Errorf("Failed to get market data: %w", err)
	}

	// Position
	positions, err := b.getPositions()
	if err != nil {
		return fmt.Errorf("Failed to get position details: %w", err)
	}

	// If we have not traded yet then we won't have a position
	if len(positions) == 0 {
		b.openVolume = 0
	} else {
		if len(positions) != 1 {
			return errors.New("One position item required")
		}
		b.openVolume = positions[0].OpenVolume
	}
	return nil
}

func convertUint256(valueStr string) (value *num.Uint, err error) {
	value, overflowed := num.UintFromString(valueStr, 10)
	if overflowed {
		err = errors.New("invalid uint256, needs to be base 10")
	}
	return
}

func (b *Bot) getAccount(typ vega.AccountType) (*num.Uint, error) {
	response, err := b.node.ListAccounts(&dataapipbv2.ListAccountsRequest{
		Filter: &dataapipbv2.AccountFilter{
			PartyIds:     []string{b.walletPubKey},
			AssetId:      b.settlementAssetID,
			AccountTypes: []vega.AccountType{typ},
			MarketIds:    []string{b.market.Id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(response.Accounts.Edges) == 0 || response.Accounts.Edges[0].Node == nil {
		b.log.WithFields(log.Fields{
			"type": typ,
		}).Debug("zero accounts for party")
		return num.Zero(), nil
	}
	if len(response.Accounts.Edges) > 1 {
		return nil, fmt.Errorf("too many accounts for party: %d", len(response.Accounts.Edges))
	}

	return convertUint256(response.Accounts.Edges[0].Node.Balance)
}

// getAccountGeneral get this bot's general account balance.
func (b *Bot) getAccountGeneral() error {
	balance, err := b.getAccount(vega.AccountType_ACCOUNT_TYPE_GENERAL)
	if err != nil {
		return fmt.Errorf("failed to get general account balance: %w", err)
	}
	b.balanceGeneral = balance
	return nil
}

// getAccountMargin get this bot's margin account balance.
func (b *Bot) getAccountMargin() error {
	balance, err := b.getAccount(vega.AccountType_ACCOUNT_TYPE_MARGIN)
	if err != nil {
		return fmt.Errorf("failed to get margin account balance: %w", err)
	}
	b.balanceMargin = balance
	return nil
}

// getAccountBond get this bot's bond account balance.
func (b *Bot) getAccountBond() error {
	b.balanceBond = num.Zero()
	balance, err := b.getAccount(vega.AccountType_ACCOUNT_TYPE_BOND)
	if err != nil {
		return fmt.Errorf("failed to get bond account balance: %w", err)
	}
	b.balanceBond = balance
	return nil
}

// getPositions get this bot's positions.
func (b *Bot) getPositions() ([]*vega.Position, error) {
	response, err := b.node.ListPositions(&dataapipbv2.ListPositionsRequest{
		PartyId:  b.walletPubKey,
		MarketId: b.market.Id,
	})
	if err != nil {
		return nil, err
	}

	result := []*vega.Position{}
	for _, partyEdge := range response.Positions.Edges {
		if partyEdge.Node == nil {
			continue
		}

		result = append(result, partyEdge.Node)
	}

	return result, nil
}

// getMarketData gets the latest info about the market.
func (b *Bot) getMarketData() error {
	// TODO: Add support for pages
	response, err := b.node.GetLatestMarketData(&dataapipbv2.GetLatestMarketDataRequest{
		MarketId: b.market.Id,
	})
	if err != nil {
		return fmt.Errorf("failed to get market data (ID:%s): %w", b.market.Id, err)
	}
	b.marketData = response.MarketData
	b.currentPrice, err = convertUint256(response.MarketData.StaticMidPrice)
	if err != nil {
		return fmt.Errorf("failed to get current price from market data (ID:%s): price=\"%s\" %w", b.market.Id, response.MarketData.StaticMidPrice, err)
	}
	return nil
}
