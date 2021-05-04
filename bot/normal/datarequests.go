package normal

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

// As the streaming service only gives us data when it changes
// we need to look up the initial data manually
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
	if len(positions) != 1 {
		return errors.New("One position item required")
	}
	b.openVolume = positions[0].OpenVolume
	return nil
}

// getAccountGeneral get this bot's general account balance.
func (b *Bot) getAccountGeneral() error {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		// MarketId: general account is not per market
		PartyId: b.walletPubKeyHex,
		Asset:   b.settlementAsset,
		Type:    proto.AccountType_ACCOUNT_TYPE_GENERAL,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get general account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero general accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many general accounts for party: %d", len(response.Accounts))
	}
	b.balanceGeneral = response.Accounts[0].Balance
	return nil
}

// getAccountMargin get this bot's margin account balance.
func (b *Bot) getAccountMargin() error {
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
		Asset:    b.settlementAsset,
		Type:     proto.AccountType_ACCOUNT_TYPE_MARGIN,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get margin account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero margin accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many margin accounts for party: %d", len(response.Accounts))
	}
	b.balanceMargin = response.Accounts[0].Balance
	return nil
}

// getAccountBond get this bot's bond account balance.
func (b *Bot) getAccountBond() error {
	b.balanceBond = 0
	response, err := b.node.PartyAccounts(&api.PartyAccountsRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
		Asset:    b.settlementAsset,
		Type:     proto.AccountType_ACCOUNT_TYPE_BOND,
	})
	if err != nil {
		return errors.Wrap(err, "failed to get bond account")
	}
	if len(response.Accounts) == 0 {
		return errors.Wrap(err, "found zero bond accounts for party")
	}
	if len(response.Accounts) > 1 {
		return fmt.Errorf("found too many bond accounts for party: %d", len(response.Accounts))
	}
	b.balanceBond = response.Accounts[0].Balance
	return nil
}

// getPositions get this bot's positions.
func (b *Bot) getPositions() ([]*proto.Position, error) {
	response, err := b.node.PositionsByParty(&api.PositionsByPartyRequest{
		PartyId:  b.walletPubKeyHex,
		MarketId: b.market.Id,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get positions by party")
	}
	return response.Positions, nil
}

// getMarketData gets the latest info about the market
func (b *Bot) getMarketData() error {
	response, err := b.node.MarketDataByID(&api.MarketDataByIDRequest{
		MarketId: b.market.Id,
	})
	if err != nil {
		return errors.Wrap(err, "Failed to get market data by id")
	}
	b.marketData = response.MarketData
	return nil
}
