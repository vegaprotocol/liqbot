// Command liqbot runs one application instance which may contain many bots that
// provide liquidity. Each bot connects to one Vega node and submits orders to
// one market.
//
//     $ make install
//     $ $GOPATH/bin/liqbot -config=config.yml
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"code.vegaprotocol.io/liqbot/config"

	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

var (
	// Version is set at build time using: -ldflags "-X main.Version=someversion"
	Version = "no_version_set"

	// VersionHash is set at build time using: -ldflags "-X main.VersionHash=somehash"
	VersionHash = "no_hash_set"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	var configName string
	var configVersion bool
	flag.StringVar(&configName, "config", "", "Configuration YAML file")
	flag.BoolVar(&configVersion, "version", false, "Show version")
	flag.Parse()

	if configVersion {
		fmt.Printf("version %v (%v)\n", Version, VersionHash)
		return
	}

	var cfg config.Config
	err := configor.Load(&cfg, configName)
	// https://github.com/jinzhu/configor/issues/40
	if err != nil && !strings.Contains(err.Error(), "should be struct") {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("Failed to read config")
	}
	err = config.CheckConfig(&cfg)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("Config checks failed")
	}

	err = config.ConfigureLogging(cfg.Server)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err.Error(),
		}).Fatal("Failed to load config")
	}

	log.WithFields(log.Fields{
		"version": Version,
		"hash":    VersionHash,
	}).Info("Version")

	// TODO: Instantiate service
}
