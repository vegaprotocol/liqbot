package token

import (
	"context"
	"fmt"
	"math/big"
	"time"

	log "github.com/sirupsen/logrus"

	vgethereum "code.vegaprotocol.io/shared/libs/ethereum"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/types/num"
)

type Service struct {
	client                  *vgethereum.Client
	vegaPubKey              string
	erc20BridgeAddress      common.Address
	stakingBridgeAddress    common.Address
	erc20TokenAddress       common.Address
	vegaTokenAddress        common.Address
	contractOwnerAddress    common.Address
	contractOwnerPrivateKey string
	syncTimeout             *time.Duration
	log                     *log.Entry
}

func NewService(conf *config.TokenConfig, baseTokenAddress, vegaPubKey string) (*Service, error) {
	ctx := context.Background()

	var syncTimeout time.Duration
	if conf.SyncTimeoutSec != 0 {
		syncTimeout = time.Duration(conf.SyncTimeoutSec) * time.Second
	}

	client, err := vgethereum.NewClient(ctx, conf.EthereumAPIAddress, conf.ChainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ethereum client: %w", err)
	}

	return &Service{
		client:                  client,
		vegaPubKey:              vegaPubKey,
		erc20BridgeAddress:      common.HexToAddress(conf.Erc20BridgeAddress),
		stakingBridgeAddress:    common.HexToAddress(conf.StakingBridgeAddress),
		erc20TokenAddress:       common.HexToAddress(baseTokenAddress),
		vegaTokenAddress:        common.HexToAddress(conf.VegaTokenAddress),
		contractOwnerAddress:    common.HexToAddress(conf.ContractOwnerAddress),
		contractOwnerPrivateKey: conf.ContractOwnerPrivateKey,
		syncTimeout:             &syncTimeout,
		log:                     log.WithFields(log.Fields{"service": "Token"}),
	}, nil
}

func (s *Service) Stake(ctx context.Context, amount *num.Uint) error {
	stakingBridge, err := s.client.NewStakingBridgeSession(ctx, s.contractOwnerPrivateKey, s.stakingBridgeAddress, s.syncTimeout)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	vegaToken, err := s.client.NewBaseTokenSession(ctx, s.contractOwnerPrivateKey, s.vegaTokenAddress, s.syncTimeout)
	if err != nil {
		return fmt.Errorf("failed to create vega token: %w", err)
	}

	if err = s.mintToken(vegaToken, s.contractOwnerAddress, amount.BigInt()); err != nil {
		return fmt.Errorf("failed to mint vegaToken: %w", err)
	}

	if err = s.approveAndStakeToken(vegaToken, stakingBridge, amount.BigInt()); err != nil {
		return fmt.Errorf("failed to approve and stake token on staking bridge: %w", err)
	}

	s.log.Debug("Stake request sent")

	return nil
}

func (s *Service) Deposit(ctx context.Context, amount *num.Uint) error {
	erc20Token, err := s.client.NewBaseTokenSession(ctx, s.contractOwnerPrivateKey, s.erc20TokenAddress, s.syncTimeout)
	if err != nil {
		return fmt.Errorf("failed to create ERC20 token: %w", err)
	}

	erc20bridge, err := s.client.NewERC20BridgeSession(ctx, s.contractOwnerPrivateKey, s.erc20BridgeAddress, s.syncTimeout)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	if err = s.mintToken(erc20Token, s.contractOwnerAddress, amount.BigInt()); err != nil {
		return fmt.Errorf("failed to mint erc20Token: %w", err)
	}

	if err = s.approveAndDepositToken(erc20Token, erc20bridge, amount.BigInt()); err != nil {
		return fmt.Errorf("failed to approve and deposit token on erc20 bridge: %w", err)
	}

	s.log.Debug("Deposit request sent")

	return nil
}

type token interface {
	MintSync(to common.Address, amount *big.Int) (*types.Transaction, error)
	ApproveSync(spender common.Address, value *big.Int) (*types.Transaction, error)
	Address() common.Address
	Name() (string, error)
}

func (s *Service) mintToken(token token, address common.Address, amount *big.Int) error {
	name, err := token.Name()
	if err != nil {
		return fmt.Errorf("failed to get name of token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": address,
		}).Debug("Minting new token")

	if _, err = token.MintSync(address, amount); err != nil {
		return fmt.Errorf("failed to call Mint contract: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": address,
		}).Debug("Token minted")

	return nil
}

func (s *Service) approveAndDepositToken(token token, bridge *vgethereum.ERC20BridgeSession, amount *big.Int) error {
	name, err := token.Name()
	if err != nil {
		return fmt.Errorf("failed to get name of token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Approving token")

	if _, err = token.ApproveSync(bridge.Address(), amount); err != nil {
		return fmt.Errorf("failed to approve token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Depositing asset")

	vegaPubKeyByte32, err := vgethereum.HexStringToByte32Array(s.vegaPubKey)
	if err != nil {
		return err
	}

	if _, err = bridge.DepositAssetSync(token.Address(), amount, vegaPubKeyByte32); err != nil {
		return fmt.Errorf("failed to deposit asset: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Token deposited")

	return nil
}

func (s *Service) approveAndStakeToken(token token, bridge *vgethereum.StakingBridgeSession, amount *big.Int) error {
	name, err := token.Name()
	if err != nil {
		return fmt.Errorf("failed to get name of token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": bridge.Address(),
		}).Debug("Approving token")

	if _, err = token.ApproveSync(bridge.Address(), amount); err != nil {
		return fmt.Errorf("failed to approve token: %w", err)
	}

	vegaPubKeyByte32, err := vgethereum.HexStringToByte32Array(s.vegaPubKey)
	if err != nil {
		return err
	}

	s.log.WithFields(
		log.Fields{
			"token":      name,
			"amount":     amount,
			"vegaPubKey": s.vegaPubKey,
		}).Debug("Staking asset")

	if _, err = bridge.Stake(amount, vegaPubKeyByte32); err != nil {
		return fmt.Errorf("failed to stake asset: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":      name,
			"amount":     amount,
			"vegaPubKey": s.vegaPubKey,
		}).Debug("Token staked")

	return nil
}
