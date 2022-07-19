package seed

import (
	"context"
	"fmt"
	"math/big"

	vgethereum "code.vegaprotocol.io/shared/libs/ethereum"
	log "github.com/sirupsen/logrus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"code.vegaprotocol.io/liqbot/config"
)

type Service struct {
	ethereumAPIAddress      string
	erc20BridgeAddress      common.Address
	stakingBridgeAddress    common.Address
	erc20TokenAddress       common.Address
	vegaTokenAddress        common.Address
	contractOwnerAddress    common.Address
	contractOwnerPrivateKey string
	amount                  *big.Int
	log                     *log.Entry
}

func NewService(conf *config.SeedConfig) (Service, error) {
	if conf == nil {
		return Service{}, fmt.Errorf("config is nil")
	}

	return Service{
		ethereumAPIAddress:      conf.EthereumAPIAddress,
		erc20BridgeAddress:      common.HexToAddress(conf.Erc20BridgeAddress),
		stakingBridgeAddress:    common.HexToAddress(conf.StakingBridgeAddress),
		erc20TokenAddress:       common.HexToAddress(conf.ERC20TokenAddress),
		vegaTokenAddress:        common.HexToAddress(conf.VegaTokenAddress),
		contractOwnerAddress:    common.HexToAddress(conf.ContractOwnerAddress),
		contractOwnerPrivateKey: conf.ContractOwnerPrivateKey,
		amount:                  big.NewInt(conf.Amount),
		log:                     log.WithFields(log.Fields{"service": "seed"}),
	}, nil
}

func (s Service) SeedStakeDeposit(ctx context.Context, vegaPubKey string) error {
	client, err := vgethereum.NewClient(ctx, s.ethereumAPIAddress, 1440)
	if err != nil {
		return fmt.Errorf("failed to create Ethereum client: %w", err)
	}

	stakingBridge, err := client.NewStakingBridgeSession(ctx, s.contractOwnerPrivateKey, s.stakingBridgeAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	erc20bridge, err := client.NewERC20BridgeSession(ctx, s.contractOwnerPrivateKey, s.erc20BridgeAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	erc20Token, err := client.NewBaseTokenSession(ctx, s.contractOwnerPrivateKey, s.erc20TokenAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create ERC20 token: %w", err)
	}

	vegaToken, err := client.NewBaseTokenSession(ctx, s.contractOwnerPrivateKey, s.vegaTokenAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create vega token: %w", err)
	}

	if err := s.mintToken(erc20Token, s.contractOwnerAddress, s.amount); err != nil {
		return fmt.Errorf("failed to mint and show balances for erc20Token: %w", err)
	}

	if err := s.mintToken(vegaToken, s.contractOwnerAddress, s.amount); err != nil {
		return fmt.Errorf("failed to mint and show balances for vegaToken: %w", err)
	}

	if err := s.approveAndDepositToken(erc20Token, erc20bridge, s.amount, vegaPubKey); err != nil {
		return fmt.Errorf("failed to approve and deposit token on erc20 bridge: %w", err)
	}

	if err := s.approveAndStakeToken(vegaToken, stakingBridge, s.amount, vegaPubKey); err != nil {
		return fmt.Errorf("failed to approve and stake token on staking bridge: %w", err)
	}

	s.log.Debug("Seeding stake deposit completed")

	return nil
}

type token interface {
	MintSync(to common.Address, amount *big.Int) (*types.Transaction, error)
	BalanceOf(account common.Address) (*big.Int, error)
	ApproveSync(spender common.Address, value *big.Int) (*types.Transaction, error)
	Address() common.Address
}

func (s Service) mintToken(token token, address common.Address, amount *big.Int) error {
	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": address,
		}).Debug("Minting new token")

	if _, err := token.MintSync(address, amount); err != nil {
		return fmt.Errorf("failed to call Mint contract: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": address,
		}).Debug("Token minted")

	return nil
}

func (s Service) approveAndDepositToken(token token, bridge *vgethereum.ERC20BridgeSession, amount *big.Int, vegaPubKey string) error {
	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Approving token")

	if _, err := token.ApproveSync(bridge.Address(), amount); err != nil {
		return fmt.Errorf("failed to approve token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Depositing asset")

	vegaPubKeyByte32, err := vgethereum.HexStringToByte32Array(vegaPubKey)
	if err != nil {
		return err
	}

	if _, err := bridge.DepositAssetSync(token.Address(), amount, vegaPubKeyByte32); err != nil {
		return fmt.Errorf("failed to deposit asset: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Token deposited")

	return nil
}

func (s Service) approveAndStakeToken(token token, bridge *vgethereum.StakingBridgeSession, amount *big.Int, vegaPubKey string) error {
	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Approving token")

	if _, err := token.ApproveSync(bridge.Address(), amount); err != nil {
		return fmt.Errorf("failed to approve token: %w", err)
	}

	vegaPubKeyByte32, err := vgethereum.HexStringToByte32Array(vegaPubKey)
	if err != nil {
		return err
	}

	s.log.WithFields(
		log.Fields{
			"token":      token.Address(),
			"amount":     amount,
			"vegaPubKey": vegaPubKey,
		}).Debug("Staking asset")

	if _, err := bridge.Stake(amount, vegaPubKeyByte32); err != nil {
		return fmt.Errorf("failed to stake asset: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":      token.Address(),
			"amount":     amount,
			"vegaPubKey": vegaPubKey,
		}).Debug("Token staked")

	return nil
}
