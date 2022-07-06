package seed

import (
	"context"
	"fmt"
	"math/big"
	"time"

	vgethereum "code.vegaprotocol.io/shared/libs/ethereum"
	log "github.com/sirupsen/logrus"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	erc20BridgeAddress   = common.HexToAddress("0x9708FF7510D4A7B9541e1699d15b53Ecb1AFDc54")
	stakingBridgeAddress = common.HexToAddress("0x9135f5afd6F055e731bca2348429482eE614CFfA")
	tUSDCTokenAddress    = common.HexToAddress("0x1b8a1B6CBE5c93609b46D1829Cc7f3Cb8eeE23a0")
	vegaTokenAddress     = common.HexToAddress("0x67175Da1D5e966e40D11c4B2519392B2058373de")
	contractOwnerAddress = common.HexToAddress("0xEe7D375bcB50C26d52E1A4a472D8822A2A22d94F")

	contractOwnerPrivateKey = "a37f4c2a678aefb5037bf415a826df1540b330b7e471aa54184877ba901b9ef0"
)

type Service struct {
	ethereumAddress string
	log             *log.Entry
}

func NewService(ethereumAddress string) Service {
	return Service{
		ethereumAddress: ethereumAddress,
		log:             log.WithFields(log.Fields{"service": "seed"}),
	}
}

func (s Service) SeedStakeDeposit(vegaPubKey string) error {
	amount := big.NewInt(1000000000000000000)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	client, err := vgethereum.NewClient(ctx, s.ethereumAddress, 1440)
	if err != nil {
		return fmt.Errorf("failed to create Ethereum client: %w", err)
	}

	stakingBridge, err := client.NewStakingBridgeSession(ctx, contractOwnerPrivateKey, stakingBridgeAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	erc20bridge, err := client.NewERC20BridgeSession(ctx, contractOwnerPrivateKey, erc20BridgeAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create staking bridge: %w", err)
	}

	tUSDCToken, err := client.NewBaseTokenSession(ctx, contractOwnerPrivateKey, tUSDCTokenAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create tUSDC token: %w", err)
	}

	vegaToken, err := client.NewBaseTokenSession(ctx, contractOwnerPrivateKey, vegaTokenAddress, nil)
	if err != nil {
		return fmt.Errorf("failed to create vega token: %w", err)
	}

	if err := s.mintTokenAndShowBalances(tUSDCToken, contractOwnerAddress, amount); err != nil {
		return fmt.Errorf("failed to mint and show balances for tUSDCToken: %w", err)
	}

	if err := s.mintTokenAndShowBalances(vegaToken, contractOwnerAddress, amount); err != nil {
		return fmt.Errorf("failed to mint and show balances for vegaToken: %w", err)
	}

	if err := s.approveAndDepositToken(tUSDCToken, erc20bridge, amount, vegaPubKey); err != nil {
		return fmt.Errorf("failed to approve and deposit token on erc20 bridge: %w", err)
	}

	if err := s.approveAndStakeToken(vegaToken, stakingBridge, amount, vegaPubKey); err != nil {
		return fmt.Errorf("failed to approve and stake token on staking bridge: %w", err)
	}

	s.log.Debug("Seeding stake deposit completed")

	return nil
}

type token interface {
	Mint(to common.Address, amount *big.Int) (*types.Transaction, error)
	MintSync(to common.Address, amount *big.Int) (*types.Transaction, error)
	BalanceOf(account common.Address) (*big.Int, error)
	ApproveSync(spender common.Address, value *big.Int) (*types.Transaction, error)
	Address() common.Address
}

func (s Service) mintTokenAndShowBalances(token token, address common.Address, amount *big.Int) error {
	s.log.Debug("---- Minting new token")

	balance, err := token.BalanceOf(address)
	if err != nil {
		return fmt.Errorf("failed to get balance for %s: %w", address, err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"address": address,
			"balance": balance,
		}).Debug("Initial balance")

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": address,
		}).Debug("Minting new token")

	if _, err := token.MintSync(address, amount); err != nil {
		return fmt.Errorf("failed to call Mint contract: %w", err)
	}

	balance, err = token.BalanceOf(address)
	if err != nil {
		return fmt.Errorf("failed to get balance for %s: %w", address, err)
	}

	s.log.WithFields(
		log.Fields{
			"token":   token.Address(),
			"amount":  amount,
			"address": address,
		}).Debug("Balance after minting")

	s.log.Debug("---- Token minted")

	return nil
}

func (s Service) approveAndDepositToken(token token, bridge *vgethereum.ERC20BridgeSession, amount *big.Int, vegaPubKey string) error {
	s.log.Debug("---- Deposit token")

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

	s.log.Debug("---- Token deposited")

	return nil
}

func (s Service) approveAndStakeToken(token token, bridge *vgethereum.StakingBridgeSession, amount *big.Int, vegaPubKey string) error {
	s.log.Debug("---- Stake token")

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

	s.log.Debug("---- Token staked")

	return nil
}
