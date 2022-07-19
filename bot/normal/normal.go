package normal

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/vegawallet/wallets"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/types"
)

// bot represents one Normal liquidity bot.
type bot struct {
	pricingEngine PricingEngine
	node          tradingDataService
	locations     []string
	data          dataStore
	marketStream  marketStream
	walletClient  WalletClient

	config     config.BotConfig
	seedConfig *config.SeedConfig
	log        *log.Entry

	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	pausePosMgmt    chan struct{}
	pausePriceSteer chan struct{}

	walletPubKey           string // "58595a" ...
	marketID               string
	decimalPlaces          int
	settlementAssetID      string
	settlementAssetAddress string
}

// New returns a new instance of bot.
func New(botConf config.BotConfig, locations []string, seedConf *config.SeedConfig, pe PricingEngine, wc WalletClient) *bot {
	return &bot{
		config:     botConf,
		seedConfig: seedConf,
		log: log.WithFields(log.Fields{
			"bot":  botConf.Name,
			"node": locations,
		}),
		locations:       locations,
		pricingEngine:   pe,
		walletClient:    wc,
		stopPosMgmt:     make(chan bool),
		stopPriceSteer:  make(chan bool),
		pausePosMgmt:    make(chan struct{}),
		pausePriceSteer: make(chan struct{}),
	}
}

// Start starts the liquidity bot goroutine(s).
func (b *bot) Start() error {
	connTimeout := time.Duration(b.config.ConnectTimeout) * time.Millisecond
	callTimeout := time.Duration(b.config.CallTimeout) * time.Millisecond

	dataNode := node.NewDataNode(
		b.locations,
		connTimeout,
		callTimeout,
	)

	if err := dataNode.DialConnection(); err != nil {
		return fmt.Errorf("failed to connect to Vega gRPC node: %w", err)
	}

	b.node = dataNode
	b.marketStream = data.NewMarket(dataNode, b.walletPubKey)

	b.log.WithFields(log.Fields{}).Debug("Connected to Vega gRPC node")

	if err := b.setupWallet(); err != nil {
		return fmt.Errorf("failed to setup wallet: %w", err)
	}

	if err := b.setupMarket(); err != nil {
		return fmt.Errorf("failed to setup market: %w", err)
	}

	b.log.WithFields(log.Fields{
		"id":                b.marketID,
		"base/ticker":       b.config.InstrumentBase,
		"quote":             b.config.InstrumentQuote,
		"settlementAssetID": b.settlementAssetID,
	}).Info("Fetched market info")

	// Use the settlementAssetID to lookup the settlement ethereum address
	assetResponse, err := b.node.AssetByID(&dataapipb.AssetByIDRequest{Id: b.settlementAssetID})
	if err != nil {
		return fmt.Errorf("unable to look up asset details for %s", b.settlementAssetID)
	}

	erc20 := assetResponse.Asset.Details.GetErc20()
	if erc20 != nil {
		b.settlementAssetAddress = erc20.ContractAddress
	}

	store := data.NewStore()
	b.data = store
	stream := data.NewStream(dataNode, store, b.pauseChannel())

	if err = stream.InitForData(b.marketID, b.walletPubKey, b.settlementAssetID); err != nil {
		return fmt.Errorf("failed to initialise data: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer cancel()
		b.runPositionManagement(ctx)
	}()

	go func() {
		defer cancel()
		b.runPriceSteering(ctx)
	}()

	return nil
}

func (b *bot) pauseChannel() chan struct{} {
	in := make(chan struct{})
	go func() {
		for {
			select {
			case <-in:
				select {
				default:
					b.pausePosMgmt <- struct{}{}
				}
				select {
				default:
					b.pausePriceSteer <- struct{}{}
				}
			}
		}
	}()
	return in
}

// Stop stops the liquidity bot goroutine(s).
func (b *bot) Stop() {
	select {
	case <-b.stopPosMgmt:
	default:
		close(b.stopPosMgmt)
	}

	select {
	case <-b.stopPriceSteer:
	default:
		close(b.stopPriceSteer)
	}
}

// GetTraderDetails returns information relating to the trader.
func (b *bot) GetTraderDetails() string {
	jsn, _ := json.MarshalIndent(map[string]string{
		"name":                              b.config.Name,
		"pubKey":                            b.walletPubKey,
		"settlementVegaAssetID":             b.settlementAssetID,
		"settlementEthereumContractAddress": b.settlementAssetAddress,
	}, "", "  ")
	return string(jsn)
}

func (b *bot) setupWallet() error {
	ctx := context.Background()
	walletPassphrase := "123"

	if err := b.walletClient.LoginWallet(ctx, b.config.Name, walletPassphrase); err != nil {
		if strings.Contains(err.Error(), wallets.ErrWalletDoesNotExists.Error()) {
			if err = b.walletClient.CreateWallet(ctx, b.config.Name, walletPassphrase); err != nil {
				return fmt.Errorf("failed to create wallet: %w", err)
			}
			b.log.Debug("Created and logged into wallet")
		} else {
			return fmt.Errorf("failed to log into wallet: %w", err)
		}
	}

	b.log.Debug("Logged into wallet")

	if b.walletPubKey == "" {
		publicKeys, err := b.walletClient.ListPublicKeys(ctx)
		if err != nil {
			return fmt.Errorf("failed to list public keys: %w", err)
		}

		if len(publicKeys) == 0 {
			key, err := b.walletClient.GenerateKeyPair(ctx, walletPassphrase, []types.Meta{})
			if err != nil {
				return fmt.Errorf("failed to generate keypair: %w", err)
			}
			b.walletPubKey = key.Pub
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Created keypair")
		} else {
			b.walletPubKey = publicKeys[0]
			b.log.WithFields(log.Fields{"pubKey": b.walletPubKey}).Debug("Using existing keypair")
		}
	}

	b.log = b.log.WithFields(log.Fields{"pubkey": b.walletPubKey})

	return nil
}
