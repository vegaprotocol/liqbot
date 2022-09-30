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
	client               *vgethereum.Client
	vegaPubKey           string
	erc20BridgeAddress   common.Address
	stakingBridgeAddress common.Address
	syncTimeout          *time.Duration
	log                  *log.Entry
}

func NewService(conf *config.TokenConfig, vegaPubKey string) (*Service, error) {
	ctx := context.Background()

	var syncTimeout time.Duration
	if conf.SyncTimeoutSec != 0 {
		syncTimeout = time.Duration(conf.SyncTimeoutSec) * time.Second
	}

	client, err := vgethereum.NewClient(ctx, conf.EthereumAPIAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create Ethereum client: %w", err)
	}

	return &Service{
		client:               client,
		vegaPubKey:           vegaPubKey,
		erc20BridgeAddress:   common.HexToAddress(conf.Erc20BridgeAddress),
		stakingBridgeAddress: common.HexToAddress(conf.StakingBridgeAddress),
		syncTimeout:          &syncTimeout,
		log:                  log.WithFields(log.Fields{"service": "Token"}),
	}, nil
}

func (s *Service) Stake(ctx context.Context, ownerPrivateKey, ownerAddress, vegaTokenAddress string, amount *num.Uint) (*num.Uint, error) {
	return s.StakeToAddress(ctx, ownerPrivateKey, ownerAddress, vegaTokenAddress, s.vegaPubKey, amount)
}

func (s *Service) StakeToAddress(ctx context.Context, ownerPrivateKey, ownerAddress, vegaTokenAddress, vegaPubKey string, amount *num.Uint) (*num.Uint, error) {
	stakingBridge, err := s.client.NewStakingBridgeSession(ctx, ownerPrivateKey, s.stakingBridgeAddress, s.syncTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking bridge: %w", err)
	}

	vegaToken, err := s.client.NewBaseTokenSession(ctx, ownerPrivateKey, common.HexToAddress(vegaTokenAddress), s.syncTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create vega token: %w", err)
	}

	minted, err := s.mintToken(ctx, vegaToken, common.HexToAddress(ownerAddress), amount.BigInt())
	if err != nil {
		return nil, fmt.Errorf("failed to mint vegaToken: %w", err)
	}

	if err = s.approveAndStakeToken(vegaToken, vegaPubKey, stakingBridge, minted); err != nil {
		return nil, fmt.Errorf("failed to approve and stake token on staking bridge: %w", err)
	}

	s.log.Debug("Stake request sent")

	staked, overflow := num.UintFromBig(minted)
	if overflow {
		return nil, fmt.Errorf("overflow when converting minted amount to uint")
	}

	return staked, nil
}

func (s *Service) Deposit(ctx context.Context, ownerPrivateKey, ownerAddress, erc20TokenAddress string, amount *num.Uint) (*num.Uint, error) {
	erc20Token, err := s.client.NewBaseTokenSession(ctx, ownerPrivateKey, common.HexToAddress(erc20TokenAddress), s.syncTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create ERC20 token: %w", err)
	}

	erc20bridge, err := s.client.NewERC20BridgeSession(ctx, ownerPrivateKey, s.erc20BridgeAddress, s.syncTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create staking bridge: %w", err)
	}

	minted, err := s.mintToken(ctx, erc20Token, common.HexToAddress(ownerAddress), amount.BigInt())
	if err != nil {
		return nil, fmt.Errorf("failed to mint erc20Token token: %w", err)
	}

	if err = s.approveAndDepositToken(erc20Token, erc20bridge, minted); err != nil {
		return nil, fmt.Errorf("failed to approve and deposit token on erc20 bridge: %w", err)
	}

	s.log.Debug("Deposit request sent")

	deposited, overflow := num.UintFromBig(minted)
	if overflow {
		return nil, fmt.Errorf("overflow when converting minted amount to uint")
	}

	return deposited, nil
}

type token interface {
	MintSync(to common.Address, amount *big.Int) (*types.Transaction, error)
	MintRawSync(ctx context.Context, toAddress common.Address, amount *big.Int) (*big.Int, error)
	ApproveSync(spender common.Address, value *big.Int) (*types.Transaction, error)
	BalanceOf(owner common.Address) (*big.Int, error)
	GetLastTransferValueSync(ctx context.Context, signedTx *types.Transaction) (*big.Int, error)
	Address() common.Address
	Name() (string, error)
}

func (s *Service) mintToken(ctx context.Context, token token, address common.Address, amount *big.Int) (*big.Int, error) {
	name, err := token.Name()
	if err != nil {
		return nil, fmt.Errorf("failed to get name of token: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   name,
			"amount":  amount,
			"address": address,
		}).Debug("Minting new token")

	var tx *types.Transaction
	if tx, err = token.MintSync(address, amount); err == nil {
		s.log.WithFields(
			log.Fields{
				"token":   name,
				"amount":  amount,
				"address": address,
			}).Debug("Token minted")

		minted, err := token.GetLastTransferValueSync(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to get last transfer value: %w", err)
		}
		return minted, nil
	}

	s.log.WithFields(log.Fields{"error": err}).Warn("Minting token failed")

	s.log.Debug("Fallback to minting token using hack...")

	// plan B

	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute) // TODO: make configurable
	defer cancel()

	minted, err := token.MintRawSync(ctx, address, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to mint token: %w", err)
	}

	if minted.Cmp(amount) < 0 {
		s.log.WithFields(
			log.Fields{
				"minted": minted,
				"amount": amount,
			}).Warning("Minted amount is less than expected")
	}

	return minted, nil
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

func (s *Service) approveAndStakeToken(token token, vegaPubKey string, bridge *vgethereum.StakingBridgeSession, amount *big.Int) error {
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

	vegaPubKeyByte32, err := vgethereum.HexStringToByte32Array(vegaPubKey)
	if err != nil {
		return err
	}

	s.log.WithFields(
		log.Fields{
			"token":      name,
			"amount":     amount,
			"vegaPubKey": vegaPubKey,
		}).Debug("Staking asset")

	if _, err = bridge.Stake(amount, vegaPubKeyByte32); err != nil {
		return fmt.Errorf("failed to stake asset: %w", err)
	}

	s.log.WithFields(
		log.Fields{
			"token":      name,
			"amount":     amount,
			"vegaPubKey": vegaPubKey,
		}).Debug("Token staked")

	return nil
}
