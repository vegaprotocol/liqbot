package bot

import (
	"errors"
	"fmt"

	"code.vegaprotocol.io/liqbot/bot/normal"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/wallet"
)

// New returns a new Bot instance.
func New(
	botConf config.BotConfig,
	conf config.Config,
	pricing types.PricingEngine,
	whale types.WhaleService,
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
	whale types.WhaleService,
) (types.Bot, error) {
	dataNode := node.NewDataNode(
		conf.Locations,
		conf.CallTimeoutMills,
	)

	marketStream := data.NewMarketStream(dataNode)
	streamData := data.NewStreamData(dataNode)
	botWallet := wallet.NewClient(conf.Wallet.URL)

	return normal.New(
		botConf,
		conf.VegaAssetID,
		dataNode,
		marketStream,
		streamData,
		pricing,
		botWallet,
		whale,
	), nil
}
