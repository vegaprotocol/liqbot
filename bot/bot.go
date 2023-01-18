package bot

import (
	"context"
	"errors"
	"fmt"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/market"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/account"
	"code.vegaprotocol.io/shared/libs/node"
	btypes "code.vegaprotocol.io/shared/libs/types"
	"code.vegaprotocol.io/shared/libs/wallet"
	"code.vegaprotocol.io/vega/logging"
)

// New returns a new Bot instance.
func New(
	log *logging.Logger,
	botConf config.BotConfig,
	conf config.Config,
	pricing types.PricingEngine,
	whale account.CoinProvider,
) (types.Bot, error) {
	switch botConf.Strategy {
	case config.BotStrategyNormal:
		bot, err := newNormalBot(log, botConf, conf, pricing, whale)
		if err != nil {
			return nil, fmt.Errorf("failed to create normal bot '%s': %w", botConf.Name, err)
		}
		return bot, nil
	default:
		return nil, errors.New("unrecognised bot strategy")
	}
}

func newNormalBot(
	log *logging.Logger,
	botConf config.BotConfig,
	conf config.Config,
	pricing types.PricingEngine,
	whale account.CoinProvider,
) (types.Bot, error) {
	dataNode := node.NewDataNode(
		conf.Locations,
		conf.CallTimeoutMills,
	)

	log.Debug("Attempting to connect to Vega gRPC node...")
	ctx := context.Background()
	dataNode.MustDialConnection(ctx) // blocking

	conf.Wallet.Name = botConf.Name
	conf.Wallet.Passphrase = "supersecret"
	botWallet, err := wallet.NewWalletV2Service(log, conf.Wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot wallet: %w", err)
	}

	pubKey := botWallet.PublicKey()
	pauseCh := make(chan btypes.PauseSignal)
	accountStream := account.NewStream(log, botConf.Name, dataNode, pauseCh)
	marketStream := market.NewStream(log, botConf.Name, pubKey, dataNode, pauseCh)
	accountService := account.NewService(log, botConf.Name, pubKey, accountStream, whale)
	marketService := market.NewService(log, dataNode, botWallet, pricing, accountService, marketStream, botConf, conf.VegaAssetID)
	bot := normal.New(log, botConf, conf.VegaAssetID, accountService, marketService, pauseCh)

	return bot, nil
}
