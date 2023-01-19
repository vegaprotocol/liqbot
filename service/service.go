package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"

	"code.vegaprotocol.io/liqbot/bot"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/pricing"
	"code.vegaprotocol.io/liqbot/types"
	"code.vegaprotocol.io/shared/libs/account"
	"code.vegaprotocol.io/shared/libs/erc20"
	"code.vegaprotocol.io/shared/libs/faucet"
	"code.vegaprotocol.io/shared/libs/node"
	"code.vegaprotocol.io/shared/libs/wallet"
	"code.vegaprotocol.io/shared/libs/whale"
	"code.vegaprotocol.io/vega/logging"
)

// Bot is the generic bot interface.
//
//go:generate go run github.com/golang/mock/mockgen -destination mocks/bot_mock.go -package mocks code.vegaprotocol.io/liqbot/service Bot
type Bot interface {
	Start() error
	Stop()
	GetTraderDetails() string
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
	log *logging.Logger

	config config.Config

	bots   map[string]Bot
	botsMu sync.Mutex

	server *http.Server
}

// NewService creates a new service instance (with optional mocks for test purposes).
func NewService(log *logging.Logger, config config.Config) (*Service, error) {
	s := &Service{
		Router: httprouter.New(),
		log:    log.Named("service"),

		config: config,
		bots:   make(map[string]Bot),
	}

	pricingEngine := pricing.NewEngine(*config.Pricing)

	whaleService, err := getWhale(log, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create whale service: %w", err)
	}

	if err = s.initBots(log, pricingEngine, whaleService); err != nil {
		return nil, fmt.Errorf("failed to initialise bots: %s", err.Error())
	}

	s.addRoutes()
	s.server = s.getServer()

	return s, nil
}

func getWhale(log *logging.Logger, config config.Config) (*whale.Service, error) {
	dataNode := node.NewDataNode(
		config.Locations,
		config.CallTimeoutMills,
	)

	ctx := context.Background()
	log.Info("Attempting to connect to a node...")
	dataNode.MustDialConnection(ctx)
	log.Info("Connected to a node")

	faucetURL, err := url.Parse(config.Whale.FaucetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse faucet URL: %w", err)
	}

	faucetService := faucet.New(*faucetURL)
	whaleWallet, err := wallet.NewWalletV2Service(log, config.Whale.Wallet)
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	tokenService, err := erc20.NewService(log, config.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to setup token service: %w", err)
	}

	streamWhale := account.NewStream(log, "provider-whale", dataNode, nil)
	provider := whale.NewProvider(log, dataNode, tokenService, streamWhale, config.Whale)
	pubKey := whaleWallet.PublicKey()
	accountService := account.NewService(log, "whale", pubKey, streamWhale, provider)
	return whale.NewService(log, dataNode, whaleWallet, accountService, streamWhale, faucetService, config.Whale), nil
}

func (s *Service) addRoutes() {
	s.GET("/status", s.Status) // TODO
	s.GET("/traders-settlement", s.TradersSettlement)
	// TODO: add bots to create and maintain more markets
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
	s.log.With(logging.String("listen", s.config.Server.Listen)).Info("Listening")
	return s.server.ListenAndServe()
}

// Stop stops the HTTP service.
func (s *Service) Stop() {
	wait := time.Duration(3) * time.Second
	s.log.With(logging.String("listen", s.config.Server.Listen)).Info("Shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	err := s.server.Shutdown(ctx)
	if err != nil {
		s.log.Error("Server shutdown failed", logging.Error(err))
	}
}

func (s *Service) initBots(log *logging.Logger, pricingEngine types.PricingEngine, whaleService account.CoinProvider) error {
	for _, botcfg := range s.config.Bots {
		if err := s.initBot(log, pricingEngine, botcfg, whaleService); err != nil {
			return fmt.Errorf("failed to initialise bot '%s': %w", botcfg.Name, err)
		}
	}

	return nil
}

func (s *Service) initBot(log *logging.Logger, pricingEngine types.PricingEngine, botcfg config.BotConfig, whaleService account.CoinProvider) error {
	log = log.Named(fmt.Sprintf("bot: %s", botcfg.Name))

	log.With(logging.String("strategy", botcfg.StrategyDetails.String())).Debug("read strategy config")

	b, err := bot.New(log, botcfg, s.config, pricingEngine, whaleService)
	if err != nil {
		return fmt.Errorf("failed to create bot %s: %w", botcfg.Name, err)
	}

	s.botsMu.Lock()
	defer s.botsMu.Unlock()

	s.bots[botcfg.Name] = b

	log.Info("bot initialised")

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
