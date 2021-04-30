package normal

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto"
	"github.com/vegaprotocol/api/grpc/clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

func (b *Bot) waitForGeneralAccountBalance() {
	sleepTime := b.strategy.PosManagementSleepMilliseconds
	for {
		if b.balanceGeneral > 0 {
			b.log.WithFields(log.Fields{
				"general": b.balanceGeneral,
			}).Debug("Fetched general balance")
			break
		} else {
			b.log.WithFields(log.Fields{
				"asset": b.settlementAsset,
			}).Warning("Waiting for positive general balance")
		}

		if sleepTime < 9000 {
			sleepTime += 1000
		}
		time.Sleep(time.Duration(sleepTime) * time.Millisecond)
	}
}

// As the streaming service only gives us data when it changes
// we need to look up the initial data manually
func (b *Bot) lookupInitialValues() error {
	// Collateral
	err := b.getAccountGeneral()
	if err != nil {
		return err
	}
	err = b.getAccountMargin()
	if err != nil {
		return err
	}
	err = b.getAccountBond()
	if err != nil {
		return err
	}

	// Position
	positions, err := b.getPositions()
	if err != nil {
		return err
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
