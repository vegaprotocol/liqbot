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
	dataNode.MustDialConnection(context.Background()) // blocking

	botWallet := wallet.NewClient(conf.Wallet.URL)
	accountService := account.NewAccountService(botConf.Name, dataNode, botConf.SettlementAssetID, whale)
	marketService := market.NewService(botConf.Name, dataNode, botWallet, pricing, accountService, botConf, conf.VegaAssetID)

	return normal.New(
		botConf,
		conf.VegaAssetID,
		botWallet,
		accountService,
		marketService,
	), nil
}
