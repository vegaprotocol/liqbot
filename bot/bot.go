package bot

import (
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/market"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/account"
	"code.vegaprotocol.io/shared/libs/node"
	btypes "code.vegaprotocol.io/shared/libs/types"
	"code.vegaprotocol.io/shared/libs/wallet"
)

// New returns a new Bot instance.
func New(
	botConf config.BotConfig,
	conf config.Config,
	pricing types.PricingEngine,
	whale account.CoinProvider,
) (types.Bot, error) {
	switch botConf.Strategy {
	case config.BotStrategyNormal:
		bot, err := newNormalBot(botConf, conf, pricing, whale)
		if err != nil {
			return nil, fmt.Errorf("failed to create normal bot '%s': %w", botConf.Name, err)
		}
		return bot, nil
	default:
		return nil, errors.New("unrecognised bot strategy")
	}
}

func newNormalBot(
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
	botWallet, err := wallet.NewWalletV2Service(conf.Wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot wallet: %w", err)
	}

	pubKey := botWallet.PublicKey()
	pauseCh := make(chan btypes.PauseSignal)
	accountStream := account.NewStream(botConf.Name, dataNode, pauseCh)
	marketStream := market.NewStream(botConf.Name, pubKey, dataNode, pauseCh)
	accountService := account.NewService(botConf.Name, pubKey, botConf.SettlementAssetID, accountStream, whale)
	marketService := market.NewService(dataNode, botWallet, pricing, accountService, marketStream, botConf, conf.VegaAssetID)
	bot := normal.New(botConf, conf.VegaAssetID, accountService, marketService, pauseCh)

	return bot, nil
}
