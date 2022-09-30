package bot

import (
	"context"
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/account"
	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/market"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/wallet"
)

// New returns a new Bot instance.
func New(
	botConf config.BotConfig,
	conf config.Config,
	pricing types.PricingEngine,
	whale types.CoinProvider,
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
	whale types.CoinProvider,
) (types.Bot, error) {
	dataNode := node.NewDataNode(
		conf.Locations,
		conf.CallTimeoutMills,
	)

	log.Debug("Attempting to connect to Vega gRPC node...")
	dataNode.MustDialConnection(context.Background()) // blocking

	botWallet := wallet.NewClient(conf.Wallet.URL)
	depositStream := data.NewAccountStream(dataNode)
	accountService := account.NewAccountService(botConf.Name, botConf.SettlementAssetID, depositStream, whale)

	marketStream := data.NewMarketStream(dataNode)
	marketService := market.NewService(marketStream, dataNode, botWallet, pricing, accountService, botConf, conf.VegaAssetID)

	return normal.New(
		botConf,
		conf.VegaAssetID,
		botWallet,
		accountService,
		marketService,
	), nil
}
