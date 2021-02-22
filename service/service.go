package service

import (
	"context"
	"encoding/json"
	// "io/ioutil"
	"net/http"
	// "net/url"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.vegaprotocol.io/liqbot/bot"
	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/core"
	"code.vegaprotocol.io/liqbot/pricing"

	"code.vegaprotocol.io/go-wallet/wallet"
	ppconfig "code.vegaprotocol.io/priceproxy/config"
	ppservice "code.vegaprotocol.io/priceproxy/service"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

// PricingEngine is the source of price information from the price proxy.
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

	bots   map[string]*bot.LiqBot
	botsMu sync.Mutex

	nodes   map[string]core.Node
	nodesMu sync.Mutex

	pricingEngine PricingEngine
	server        *http.Server
	walletServer  wallet.WalletHandler
}

// NewService creates a new service instance (with optional mocks for test purposes).
func NewService(config config.Config, pe PricingEngine, ws wallet.WalletHandler) (s *Service, err error) {
	if pe == nil {
		pe = pricing.NewEngine(*config.Pricing)
	}
	if ws == nil {
		// Use internal WalletHandler
		if !strings.HasPrefix(config.Wallet.RootPath, "/") {
			config.Wallet.RootPath = filepath.Join(os.Getenv("HOME"), config.Wallet.RootPath)
		}
		if err = ensureDir(config.Wallet.RootPath); err != nil {
			return
		}
		rsaKeyDir := filepath.Join(config.Wallet.RootPath, "wallet_rsa")
		if err = ensureDir(rsaKeyDir); err != nil {
			return
		}
		walletsDir := filepath.Join(config.Wallet.RootPath, "wallets")
		if err = ensureDir(walletsDir); err != nil {
			return
		}
		var log *zap.Logger
		log, err = zap.NewProduction()
		if err != nil {
			return
		}
		var pubKeyExists bool
		pubKeyExists, err = fileExists(filepath.Join(rsaKeyDir, "public.pem"))
		if err != nil {
			return
		}
		if !pubKeyExists {
			err = wallet.GenRsaKeyFiles(log, config.Wallet.RootPath, true)
			if err != nil {
				return
			}
		}
		var auth wallet.Auth
		auth, err = wallet.NewAuth(log, config.Wallet.RootPath, time.Duration(config.Wallet.TokenExpiry)*time.Second)
		if err != nil {
			return
		}
		ws = wallet.NewHandler(log, auth, config.Wallet.RootPath)
	}
	s = &Service{
		Router: httprouter.New(),

		config:        config,
		bots:          make(map[string]*bot.LiqBot),
		nodes:         make(map[string]core.Node),
		pricingEngine: pe,
		walletServer:  ws,
	}

	if err := s.initNodes(); err != nil {
		return nil, fmt.Errorf("failed to initialise nodes: %s", err.Error())
	}

	if err := s.initBots(); err != nil {
		return nil, fmt.Errorf("failed to initialise bots: %s", err.Error())
	}

	s.addRoutes()
	s.server = s.getServer()

	return
}

func (s *Service) addRoutes() {
	s.GET("/status", s.Status)
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

func (s *Service) initBots() error {
	s.botsMu.Lock()
	defer s.botsMu.Unlock()

	for _, botcfg := range s.config.Bots {
		b := bot.New(botcfg, s.pricingEngine, s.walletServer)
		s.bots[botcfg.Name] = b
		log.WithFields(log.Fields{
			"name": botcfg.Name,
		}).Info("Initialised bot")
	}

	for _, b := range s.bots {
		err := b.Start()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) initNodes() error {
	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()

	var err error
	var node core.Node

	for _, nodecfg := range s.config.Nodes {
		if nodecfg.Address == nil {
			return core.ErrNil
		}
		switch nodecfg.Address.Scheme {
		case "http":
			fallthrough
		case "https":
			bg := context.Background()
			node, err = core.NewRESTNode(bg, nodecfg.Name, *nodecfg.Address)
		case "grpc":
			bg := context.Background()
			node, err = core.NewGRPCNode(bg, nodecfg.Name, *nodecfg.Address)
		default:
			return core.ErrUnsupportedScheme
		}
		if err != nil {
			return err
		}
		s.nodes[nodecfg.Name] = node
		addr, err := node.GetAddress()
		if err != nil {
			return err
		}
		log.WithFields(log.Fields{
			"name":    nodecfg.Name,
			"address": addr.String(),
		}).Info("Initialised node")
	}
	return nil
}

// Status is an endpoint to show the service is up (always returns succeeded=true).
func (s *Service) Status(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	writeSuccess(w, SimpleResponse{Success: true}, http.StatusOK)
}

func writeSuccess(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf, _ := json.Marshal(data)
	_, _ = w.Write(buf)
}

func writeError(w http.ResponseWriter, e error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	buf, _ := json.Marshal(ErrorResponse{Error: e.Error()})
	_, _ = w.Write(buf)
}

func fileExists(path string) (bool, error) {
	fs, err := os.Stat(path)
	if err == nil {
		// fileStat -> is not a directory
		ok := !fs.IsDir()
		return ok, nil
	}
	if os.IsNotExist(err) {
		return false, nil // do not return error here
	}
	return false, err
}

func ensureDir(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.Mkdir(path, 0700)
		}
		return err
	}
	return nil
}
