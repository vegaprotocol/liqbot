package whale

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	log "github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/shared/libs/num"
	dataapipb "code.vegaprotocol.io/vega/protos/data-node/api/v2"
	"code.vegaprotocol.io/vega/protos/vega"
)

type Provider struct {
	node             dataNode
	erc20            erc20Service
	faucet           faucetClient
	slack            slacker
	ownerPrivateKeys map[string]string

	pendingDeposits map[string]pendingDeposit
	mu              sync.Mutex

	ensureBalanceCh chan ensureBalanceRequest

	walletConfig *config.WhaleConfig
	callTimeout  time.Duration
	log          *log.Entry
}

type ensureBalanceRequest struct {
	ctx     context.Context
	name    string
	address string
	assetID string
	amount  *num.Uint
}

type slacker struct {
	*slack.Client // TODO: abstract this out
	channelID     string
	enabled       bool
}

type pendingDeposit struct {
	amount    *num.Uint
	timestamp string
}

func NewProvider(
	node dataNode,
	erc20 erc20Service,
	faucet faucetClient,
	config *config.WhaleConfig,
) *Provider {
	p := &Provider{
		node:             node,
		erc20:            erc20,
		faucet:           faucet,
		ownerPrivateKeys: config.OwnerPrivateKeys,
		walletConfig:     config,
		ensureBalanceCh:  make(chan ensureBalanceRequest),
		callTimeout:      time.Duration(config.SyncTimeoutSec) * time.Second,
		slack: slacker{
			Client:    slack.New(config.SlackConfig.BotToken, slack.OptionAppLevelToken(config.SlackConfig.AppToken)),
			channelID: config.SlackConfig.ChannelID,
			enabled:   config.SlackConfig.Enabled,
		},
		log: log.WithFields(log.Fields{
			"component": "WhaleProvider",
			"whaleName": config.WalletName,
		}),
	}

	go func() {
		for req := range p.ensureBalanceCh {
			if err := p.topUpAsync(req.ctx, req.name, req.address, req.assetID, req.amount); err != nil {
				log.Errorf("Whale: failed to ensure enough funds: %s", err)
			}
		}
	}()
	return p
}

func (p *Provider) TopUpAsync(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error {
	p.ensureBalanceCh <- ensureBalanceRequest{
		ctx:     ctx,
		name:    receiverName,
		address: receiverAddress,
		assetID: assetID,
		amount:  amount,
	}

	return nil
}

func (p *Provider) topUpAsync(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error {
	// TODO: remove deposit slack request, once deposited
	if existDeposit, ok := p.getPendingDeposit(assetID); ok {
		existDeposit.amount = amount.Add(amount, existDeposit.amount)
		if p.slack.enabled {
			newTimestamp, err := p.updateDan(ctx, assetID, existDeposit.timestamp, existDeposit.amount)
			if err != nil {
				return fmt.Errorf("failed to update slack message: %s", err)
			}
			existDeposit.timestamp = newTimestamp
		}
		p.setPendingDeposit(assetID, existDeposit)
		return nil
	}

	err := p.deposit(ctx, "Whale", p.walletConfig.WalletPubKey, assetID, amount)
	if err == nil {
		return nil
	}

	p.log.WithFields(
		log.Fields{"receiverName": receiverName, "receiverAddress": receiverAddress}).
		Warningf("Failed to deposit: %s", err)

	deposit := pendingDeposit{
		amount: amount,
	}

	p.setPendingDeposit(assetID, deposit)

	if !p.slack.enabled {
		return fmt.Errorf("failed to deposit: %w", err)
	}

	p.log.Debugf("Fallback to slacking Dan...")

	deposit.timestamp, err = p.slackDan(ctx, assetID, amount)
	if err != nil {
		p.log.Errorf("Failed to slack Dan: %s", err)
		return err
	}
	p.setPendingDeposit(assetID, deposit)
	return nil
}

func (p *Provider) deposit(ctx context.Context, receiverName, receiverAddress, assetID string, amount *num.Uint) error {
	asset, err := p.node.AssetByID(ctx, &dataapipb.GetAssetRequest{
		AssetId: assetID,
	})
	if err != nil {
		return fmt.Errorf("failed to get asset id: %w", err)
	}

	if erc20 := asset.Details.GetErc20(); erc20 != nil {
		err = p.depositERC20(ctx, asset, amount)
	} else {
		return fmt.Errorf("unsupported asset type")
	}
	if err != nil {
		return fmt.Errorf("failed to deposit to address '%s', name '%s': %w", receiverAddress, receiverName, err)
	}

	return nil
}

func (p *Provider) getPendingDeposit(assetID string) (pendingDeposit, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pendingDeposits == nil {
		p.pendingDeposits = make(map[string]pendingDeposit)
		return pendingDeposit{}, false
	}

	pending, ok := p.pendingDeposits[assetID]
	return pending, ok
}

func (p *Provider) setPendingDeposit(assetID string, pending pendingDeposit) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pendingDeposits == nil {
		p.pendingDeposits = make(map[string]pendingDeposit)
	}

	p.pendingDeposits[assetID] = pending
}

func (p *Provider) StakeAsync(ctx context.Context, receiverAddress, assetID string, amount *num.Uint) error {
	asset, err := p.node.AssetByID(ctx, &dataapipb.GetAssetRequest{
		AssetId: assetID,
	})
	if err != nil {
		return fmt.Errorf("failed to get asset id: %w", err)
	}
	erc20 := asset.Details.GetErc20()
	if erc20 == nil {
		return fmt.Errorf("asset is not erc20")
	}

	ownerKey, err := p.getOwnerKeyForAsset(asset.Id)
	if err != nil {
		return fmt.Errorf("failed to get owner for key '%s': %w", receiverAddress, err)
	}

	contractAddress := asset.Details.GetErc20().ContractAddress

	added, err := p.erc20.Stake(ctx, ownerKey.privateKey, ownerKey.address, contractAddress, receiverAddress, amount)
	if err != nil {
		return fmt.Errorf("failed to stake Vega token for '%s': %w", receiverAddress, err)
	}

	// TODO: check decimal places
	if added.Int().LT(amount.Int()) {
		return fmt.Errorf("staked less than requested amount")
	}

	return nil
}

func (p *Provider) depositERC20(ctx context.Context, asset *vega.Asset, amount *num.Uint) error {
	ownerKey, err := p.getOwnerKeyForAsset(asset.Id)
	if err != nil {
		return fmt.Errorf("failed to get owner key: %w", err)
	}

	contractAddress := asset.Details.GetErc20().ContractAddress

	added, err := p.erc20.Deposit(
		ctx,
		ownerKey.privateKey,
		ownerKey.address,
		contractAddress,
		p.walletConfig.WalletPubKey,
		amount,
	)
	if err != nil {
		return fmt.Errorf("failed to add erc20 token: %w", err)
	}

	// TODO: check decimal places
	if added.Int().LT(amount.Int()) {
		return fmt.Errorf("deposited less than requested amount")
	}

	return nil
}

type key struct {
	privateKey string
	address    string
}

func (p *Provider) getOwnerKeyForAsset(assetID string) (*key, error) {
	ownerPrivateKey, ok := p.ownerPrivateKeys[assetID]
	if !ok {
		return nil, fmt.Errorf("owner private key not configured for asset '%s'", assetID)
	}

	address, err := addressFromPrivateKey(ownerPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get address from private key: %w", err)
	}

	return &key{
		privateKey: ownerPrivateKey,
		address:    address,
	}, nil
}

func addressFromPrivateKey(privateKey string) (string, error) {
	key, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert owner private key hash into ECDSA: %w", err)
	}

	publicKeyECDSA, ok := key.Public().(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return address, nil
}

const msgTemplate = `Hi @here! Whale wallet account with pub key %s needs %s coins of assetID %s, so that it can feed the hungry bots.`

func (p *Provider) slackDan(ctx context.Context, assetID string, amount *num.Uint) (string, error) {
	walletPubKey := p.walletConfig.WalletPubKey

	p.log.Debugf("Slack post @hungry-bots: wallet pub key: %s; asset id: %s; amount: %s", walletPubKey, assetID, amount.String())

	message := fmt.Sprintf(msgTemplate, walletPubKey, amount.String(), assetID)

	respChannel, respTimestamp, err := p.slack.PostMessageContext(
		ctx,
		p.slack.channelID,
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		return "", err
	}

	p.log.Debugf("Slack message successfully sent to channel %s at %s", respChannel, respTimestamp)

	time.Sleep(time.Second * 5)

	_, _, _ = p.slack.PostMessageContext(
		ctx,
		p.slack.channelID,
		slack.MsgOptionText("I can wait...", false),
	)
	return respTimestamp, nil
}

func (p *Provider) updateDan(ctx context.Context, assetID, oldTimestamp string, amount *num.Uint) (string, error) {
	walletPubKey := p.walletConfig.WalletPubKey

	p.log.Debugf("Slack update @hungry-bots: wallet pub key: %s; asset id: %s; amount: %s", walletPubKey, assetID, amount.String())

	message := fmt.Sprintf(msgTemplate, walletPubKey, amount.String(), assetID)

	respChannel, respTimestamp, _, err := p.slack.UpdateMessageContext(
		ctx,
		p.slack.channelID,
		oldTimestamp,
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		return "", err
	}

	p.log.Debugf("Slack message successfully updated in channel %s at %s", respChannel, respTimestamp)
	return respTimestamp, nil
}
