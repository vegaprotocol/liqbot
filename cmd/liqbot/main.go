package main

import (
	"flag"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jinzhu/configor"

	"code.vegaprotocol.io/liqbot/config"
	"code.vegaprotocol.io/liqbot/service"
	"code.vegaprotocol.io/vega/logging"
)

var (
	// Version is set at build time using: -ldflags "-X main.Version=someversion".
	Version = "no_version_set"

	// VersionHash is set at build time using: -ldflags "-X main.VersionHash=somehash".
	VersionHash = "no_hash_set"
)

func main() {
	var configName string
	var configVersion bool
	flag.StringVar(&configName, "config", "", "Configuration YAML file")
	flag.BoolVar(&configVersion, "version", false, "Show version")
	flag.Parse()

	log := logging.NewDevLogger()

	if configName == "" {
		log.Fatal("config was not provided")
	}

	if configVersion {
		log.With(
			logging.String("version", Version),
			logging.String("version_hash", VersionHash),
		).Info("liqbot version")
		return
	}

	rand.Seed(time.Now().UnixNano())

	var cfg config.Config
	// https://github.com/jinzhu/configor/issues/40
	if err := configor.Load(&cfg, configName); err != nil && !strings.Contains(err.Error(), "should be struct") {
		log.Fatal("Failed to read config", logging.Error(err))
	}

	if err := cfg.CheckConfig(); err != nil {
		log.Fatal("Config checks failed", logging.Error(err))
	}

	log = cfg.ConfigureLogging(log)

	log.With(
		logging.String("version", Version),
		logging.String("hash", VersionHash),
	).Info("Version")

	s, err := service.NewService(log, cfg)
	if err != nil {
		log.Fatal("Failed to create service", logging.Error(err))
	}

	go func() {
		err := s.Start()
		if err != nil && err.Error() != "http: Server closed" {
			log.With(logging.String("listen", cfg.Server.Listen)).Fatal("Could not listen", logging.Error(err))
		}
	}()
	c := make(chan os.Signal, 2)
	signal.Notify(c, syscall.SIGINT)
	signal.Notify(c, syscall.SIGTERM)
	<-c
	s.Stop()
}
