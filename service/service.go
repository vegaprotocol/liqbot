package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"code.vegaprotocol.io/liqbot/account"
	"code.vegaprotocol.io/liqbot/bot"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/data"
	"code.vegaprotocol.io/liqbot/pricing"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/liqbot/whale"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"code.vegaprotocol.io/shared/libs/erc20"
	sconfig "code.vegaprotocol.io/shared/libs/erc20/config"
	"code.vegaprotocol.io/shared/libs/faucet"
	"code.vegaprotocol.io/shared/libs/node"
	"code.vegaprotocol.io/shared/libs/wallet"
)

// Bot is the generic bot interface.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/bot_mock.go -package mocks code.vegaprotocol.io/liqbot/service Bot
type Bot interface {
	Start() error
	Stop()
	GetTraderDetails() string
}

// PricingEngine is the source of price information from the price proxy.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/pricingengine_mock.go -package mocks code.vegaprotocol.io/liqbot/service PricingEngine
type PricingEngine interface {
	GetPrice(pricecfg ppconfig.PriceConfig) (pi ppservice.PriceResponse, err error)
}

// SimpleResponse is used to show if a request succeeded or not, without giving any more detail.
type SimpleResponse struct {
	Success bool `json:"success"`
}

// ErrorResponse is used when something went wrong.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Service is the HTTP service.
type Service struct {
	*httprouter.Router

	config config.Config

	bots   map[string]Bot
	botsMu sync.Mutex

	server *http.Server
}

// NewService creates a new service instance (with optional mocks for test purposes).
func NewService(config config.Config) (*Service, error) {
	s := &Service{
		Router: httprouter.New(),

		config: config,
		bots:   make(map[string]Bot),
	}

	if err := setupLogger(config.Server); err != nil {
		return nil, fmt.Errorf("failed to setup logger: %w", err)
	}

	pricingEngine := pricing.NewEngine(*config.Pricing)

	whaleService, err := getWhale(config)
	if err != nil {
		return nil, err
	}

	if err = s.initBots(pricingEngine, whaleService); err != nil {
		return nil, fmt.Errorf("failed to initialise bots: %s", err.Error())
	}

	s.addRoutes()
	s.server = s.getServer()

	return s, nil
}

func setupLogger(conf *config.ServerConfig) error {
	level, err := log.ParseLevel(conf.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to parse log level: %w", err)
	}

	log.SetLevel(level)

	var callerPrettyfier func(*runtime.Frame) (string, string)

	if conf.LogLevel == "debug" {
		log.SetReportCaller(true)

		callerPrettyfier = func(f *runtime.Frame) (string, string) {
			filename := path.Base(f.File)
			function := strings.ReplaceAll(f.Function, "code.vegaprotocol.io/", "")
			idx := strings.Index(function, ".")
			function = fmt.Sprintf("%s/%s/%s():%d", function[:idx], filename, function[idx+1:], f.Line)
			return function, ""
		}
	}

	var formatter log.Formatter = &log.TextFormatter{
		CallerPrettyfier: callerPrettyfier,
	}

	if conf.LogFormat == "json" || conf.LogFormat == "json_pretty" {
		formatter = &log.JSONFormatter{
			PrettyPrint: conf.LogFormat == "json_pretty",
			DataKey:     "_vals",
			FieldMap: log.FieldMap{
				log.FieldKeyMsg: "_msg",
			},
			CallerPrettyfier: callerPrettyfier,
		}
	}

	log.SetFormatter(formatter)
	return nil
}

func getWhale(config config.Config) (*whale.Service, error) {
	dataNode := node.NewDataNode(
		config.Locations,
		config.CallTimeoutMills,
	)

	faucetURL, err := url.Parse(config.Whale.FaucetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse faucet URL: %w", err)
	}

	faucetService := faucet.New(*faucetURL)
	whaleWallet := wallet.NewClient(config.Wallet.URL)
	accountStream := data.NewAccountStream("whale", dataNode)

	tokenService, err := erc20.NewService((*sconfig.TokenConfig)(config.Token), config.Whale.WalletPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to setup token service: %w", err)
	}

	provider := whale.NewProvider(
		dataNode,
		tokenService,
		faucetService,
		config.Whale,
	)

	accountService := account.NewAccountService("whale", "", accountStream, provider)

	whaleService := whale.NewService(
		dataNode,
		whaleWallet,
		accountService,
		faucetService,
		config.Whale,
	)

	if err = whaleService.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start whale service: %w", err)
	}
	return whaleService, nil
}

func (s *Service) addRoutes() {
	s.GET("/status", s.Status)
	s.GET("/traders-settlement", s.TradersSettlement)
}

func (s *Service) getServer() *http.Server {
	var handler http.Handler = s // cors.AllowAll().Handler(s)

	return &http.Server{
		Addr:           s.config.Server.Listen,
		WriteTimeout:   time.Second * 15,
		ReadTimeout:    time.Second * 15,
		IdleTimeout:    time.Second * 60,
		MaxHeaderBytes: 1 << 20,
		Handler:        handler,
	}
}

// Start starts the HTTP server, and returns the server's exit error (if any).
func (s *Service) Start() error {
	log.WithFields(log.Fields{
		"listen": s.config.Server.Listen,
	}).Info("Listening")
	return s.server.ListenAndServe()
}

// Stop stops the HTTP service.
func (s *Service) Stop() {
	wait := time.Duration(3) * time.Second
	log.WithFields(log.Fields{
		"listen": s.config.Server.Listen,
	}).Info("Shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	err := s.server.Shutdown(ctx)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
		}).Info("Server shutdown failed")
	}
}

func (s *Service) initBots(pricingEngine PricingEngine, whaleService types.CoinProvider) error {
	for _, botcfg := range s.config.Bots {
		if err := s.initBot(pricingEngine, botcfg, whaleService); err != nil {
			return fmt.Errorf("failed to initialise bot '%s': %w", botcfg.Name, err)
		}
	}

	return nil
}

func (s *Service) initBot(pricingEngine PricingEngine, botcfg config.BotConfig, whaleService types.CoinProvider) error {
	log.WithFields(log.Fields{"strategy": botcfg.StrategyDetails.String()}).Debug("read strategy config")

	b, err := bot.New(botcfg, s.config, pricingEngine, whaleService)
	if err != nil {
		return fmt.Errorf("failed to create bot %s: %w", botcfg.Name, err)
	}

	s.botsMu.Lock()
	defer s.botsMu.Unlock()

	s.bots[botcfg.Name] = b

	log.WithFields(log.Fields{
		"name": botcfg.Name,
	}).Info("Initialised bot")

	if err = b.Start(); err != nil {
		return fmt.Errorf("failed to start bot %s: %w", botcfg.Name, err)
	}

	return nil
}

// Status is an endpoint to show the service is up (always returns succeeded=true).
func (s *Service) Status(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	var err error
	// TODO: check if the service is up
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
	} else {
		writeSuccess(w, SimpleResponse{Success: true}, http.StatusOK)
	}
}

// TradersSettlement is an endpoint to show details of all active traders.
func (s *Service) TradersSettlement(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	details := s.getBotsTraderDetails()
	writeString(w, details, http.StatusOK)
}

//nolint:prealloc
func (s *Service) getBotsTraderDetails() string {
	var details []string
	// Go through all the bots and ask for details
	for _, b := range s.bots {
		details = append(details, b.GetTraderDetails())
	}

	return fmt.Sprintf("[%s]", strings.Join(details, ","))
}

func writeSuccess(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf, _ := json.Marshal(data)
	_, _ = w.Write(buf)
}

func writeString(w http.ResponseWriter, str string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(str))
}

func writeError(w http.ResponseWriter, e error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf, _ := json.Marshal(ErrorResponse{Error: e.Error()})
	_, _ = w.Write(buf)
}
