package normal

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"runtime"
	"strings"
	"sync"

	dataapipb "code.vegaprotocol.io/protos/data-node/api/v1"
	"code.vegaprotocol.io/vegawallet/wallets"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/node"
	"code.vegaprotocol.io/liqbot/token"
	"code.vegaprotocol.io/liqbot/types"
)

// bot represents one Normal liquidity bot.
type bot struct {
	pricingEngine PricingEngine
	node          tradingDataService
	locations     []string
	data          dataStore
	marketStream  marketStream
	dataStream    dataStream
	walletClient  WalletClient
	tokens        tokenService

	config      config.BotConfig
	tokenConfig *config.TokenConfig
	log         *log.Entry

	stopPosMgmt     chan bool
	stopPriceSteer  chan bool
	pausePosMgmt    chan struct{}
	pausePriceSteer chan struct{}

	walletPubKey           string // "58595a" ...
	marketID               string
	decimalPlaces          int
	settlementAssetID      string
	settlementAssetAddress string
	botPaused              bool
	mu                     sync.Mutex
}

// New returns a new instance of bot.
func New(botConf config.BotConfig, locations []string, seedConf *config.TokenConfig, pe PricingEngine, wc WalletClient) *bot {
	return &bot{
		config:      botConf,
		tokenConfig: seedConf,
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
	setupLogger(false) // TODO: pretty from config?

	dataNode := node.NewDataNode(
		b.locations,
		b.config.CallTimeoutMills,
	)

	b.log.WithFields(
		log.Fields{
			"hosts": b.locations,
		}).Debug("Attempting to connect to Vega gRPC node...")

	dataNode.MustDialConnection(context.Background()) // blocking

	b.node = dataNode
	b.log = b.log.WithFields(log.Fields{"node": b.node.Target()})
	b.log.Info("Connected to Vega gRPC node")

	err := b.setupWallet()
	if err != nil {
		return fmt.Errorf("failed to setup wallet: %w", err)
	}

	pauseCh := b.pauseChannel()

	b.marketStream = data.NewMarketStream(dataNode, b.walletPubKey, pauseCh)

	assetResponse, err := b.node.AssetByID(&dataapipb.AssetByIDRequest{
		Id: b.config.QuoteTokenID,
	})
	if err != nil {
		return fmt.Errorf("failed to get asset info: %w", err)
	}

	erc20 := assetResponse.Asset.Details.GetErc20()

	quoteTokenAddress := b.config.QuoteTokenID

	if erc20 != nil {
		quoteTokenAddress = erc20.ContractAddress
	}

	b.tokens, err = token.NewService(b.tokenConfig, quoteTokenAddress, b.walletPubKey)
	if err != nil {
		return fmt.Errorf("failed to create token service: %w", err)
	}

	if err = b.setupMarket(); err != nil {
		return fmt.Errorf("failed to setup market: %w", err)
	}

	b.log.WithFields(log.Fields{
		"id":                b.marketID,
		"base/ticker":       b.config.InstrumentBase,
		"quote":             b.config.InstrumentQuote,
		"settlementAssetID": b.settlementAssetID,
	}).Info("Fetched market info")

	// Use the settlementAssetID to lookup the settlement ethereum address
	assetResponse, err = b.node.AssetByID(&dataapipb.AssetByIDRequest{Id: b.settlementAssetID})
	if err != nil {
		return fmt.Errorf("unable to look up asset details for %s: %w", b.settlementAssetID, err)
	}

	if erc20 := assetResponse.Asset.Details.GetErc20(); erc20 != nil {
		b.settlementAssetAddress = erc20.ContractAddress
	} else {
		b.settlementAssetAddress = b.settlementAssetID
	}

	store := data.NewStore()
	b.data = store

	b.dataStream, err = data.NewDataStream(b.marketID, b.walletPubKey, b.settlementAssetID, dataNode, store, pauseCh)
	if err != nil {
		return fmt.Errorf("failed to create data stream: %w", err)
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

func setupLogger(pretty bool) {
	log.SetReportCaller(true)
	log.SetFormatter(&log.JSONFormatter{
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			function := strings.ReplaceAll(f.Function, "code.vegaprotocol.io/", "")
			idx := strings.Index(function, ".")
			function = fmt.Sprintf("%s/%s/%s():%d", function[:idx], filename, function[idx+1:], f.Line)
			return function, ""
		},
		PrettyPrint: pretty,
		DataKey:     "_vals",
		FieldMap: log.FieldMap{
			log.FieldKeyMsg: "_msg",
		},
	})
}

func (b *bot) pauseChannel() chan types.PauseSignal {
	in := make(chan types.PauseSignal)
	go func() {
		for {
			select {
			case p := <-in:
				if !b.togglePause(p) {
					return
				}
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

func (b *bot) togglePause(p types.PauseSignal) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if p.Pause && !b.botPaused {
		b.log.WithFields(log.Fields{"From": p.From}).Info("Pausing bot")
		b.botPaused = true
	} else if !p.Pause && b.botPaused {
		b.log.WithFields(log.Fields{"From": p.From}).Info("Resuming bot")
		b.botPaused = false
	} else {
		return false
	}
	return true
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
			b.log.Info("Created and logged into wallet")
		} else {
			return fmt.Errorf("failed to log into wallet: %w", err)
		}
	}

	b.log.Info("Logged into wallet")

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
