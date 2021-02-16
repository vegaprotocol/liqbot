package core

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"reflect"
	"runtime"
	"time"

	"code.vegaprotocol.io/vega/proto"
	"code.vegaprotocol.io/vega/proto/api"
	log "github.com/sirupsen/logrus"
)

var (
	saneDate           = time.Date(2019, time.June, 1, 12, 0, 0, 0, time.UTC)
	nodeUpdateInterval = 30 // seconds
)

// Node specifies a generic Vega node. Implementations of Node may use any supported API.
// TBD: move elsewhere: go:generate go run github.com/golang/mock/mockgen -destination mocks/node_mock.go -package mocks code.vegaprotocol.io/liqbot/core Node
type Node interface {
	GetAddress() (url.URL, error)
	Trading
	TradingData
	GetName() (string, error)
	GetTime() (time.Time, error)
	UpdateMarketDepths() error
	UpdateMarkets() error
	UpdateTime() error
}

// Trading specifies methods exposing node functions relevant to trading.
type Trading interface {
	PrepareSubmitOrder(*api.PrepareSubmitOrderRequest) (*api.PrepareSubmitOrderResponse, error)
	SubmitTransaction(req *api.SubmitTransactionRequest) (*api.SubmitTransactionResponse, error)
}

// TradingData specifies methods exposing node functions relevant to trading data.
type TradingData interface {
	GetMarket(marketIDorName string) (proto.Market, error)
	GetMarketDepth(marketID string) (proto.MarketDepth, error)
	GetMarketDepths() (map[string]proto.MarketDepth, error)
	GetMarkets() (map[string]proto.Market, error)
}

type nodeFunc func(n Node) error

func updateMarkets(n Node) error {
	return n.UpdateMarkets()
}

func updateMarketDepths(n Node) error {
	return n.UpdateMarketDepths()
}

func updateTime(n Node) error {
	return n.UpdateTime()
}

func updateNode(n Node) {
	var err error
	nodename, _ := n.GetName()
	backoff := 1
	for {
		for _, update := range []nodeFunc{
			updateTime,
			updateMarkets,
			updateMarketDepths,
		} {
			err = update(n)
			if err != nil {
				break
			}
		}
		if err == nil {
			backoff = 1
			log.WithFields(log.Fields{
				"backoff": backoff,
				"node":    nodename,
			}).Debug("Updated node")
		} else {
			// Set a maximum sleep time of 2 minutes
			if backoff*2*nodeUpdateInterval < 120 {
				backoff *= 2
			}
			log.WithFields(log.Fields{
				"backoff": backoff,
				"error":   err.Error(),
				"node":    nodename,
			}).Warning("Failed to update node")
		}
		time.Sleep(time.Duration(backoff*nodeUpdateInterval) * time.Second)
	}
}

func checkTime(n Node) (err error) {
	err = n.UpdateTime()
	if err != nil {
		return
	}
	var t time.Time
	t, err = n.GetTime()
	if err != nil {
		return
	}
	if !t.After(saneDate) {
		return ErrInsaneDate
	}
	return
}

func checkMarkets(n Node) (err error) {
	err = n.UpdateMarkets()
	if err != nil {
		return
	}
	var mkts map[string]proto.Market
	mkts, err = n.GetMarkets()
	if err != nil {
		return
	}
	if len(mkts) == 0 {
		return ErrNoMarkets
	}
	return
}

func checkMarketDepths(n Node) (err error) {
	err = n.UpdateMarketDepths()
	if err != nil {
		return fmt.Errorf("failed to update market depths: %v", err)
	}
	var mktDepths map[string]proto.MarketDepth
	mktDepths, err = n.GetMarketDepths()
	if err != nil {
		return fmt.Errorf("failed to get market depths: %v", err)
	}
	if len(mktDepths) == 0 {
		return ErrNoMarketDepth
	}
	return
}

func waitForNode(ctx context.Context, n Node) (err error) {
	nodename, _ := n.GetName()
	backoff := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			for _, check := range []nodeFunc{
				checkTime,
				checkMarkets,
				checkMarketDepths,
			} {
				err = check(n)
				if err != nil {
					log.WithFields(log.Fields{
						"backoff": backoff,
						"func":    getFunctionName(check),
						"error":   err.Error(),
						"node":    nodename,
					}).Debug("Still waiting for Node")
					break
				}
			}
			if err == nil {
				return
			}
			// Set a maximum sleep time of 10 seconds (backoff*2*50 >= 10000)
			if backoff >= 100 {
				return fmt.Errorf("failed while waiting for node: %s", nodename)
			}
			backoff *= 2
			time.Sleep(time.Duration(50*backoff) * time.Millisecond)
		}
	}
}

// MarketDepthFromMarketDepthResponse converts a MarketDepthResponse into a MarketDepth
func MarketDepthFromMarketDepthResponse(mdr *api.MarketDepthResponse) *proto.MarketDepth {
	return &proto.MarketDepth{
		MarketId: mdr.MarketId,
		Buy:      mdr.Buy,
		Sell:     mdr.Sell,
		// LastTrade is dropped: present in MarketDepthResponse but not MarketDepth
	}
}

// RandLetters returns a string of length 26, with each uppercase letter appearing once.
func RandLetters() string {
	letters := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	shuffle := func(i, j int) { letters[i], letters[j] = letters[j], letters[i] }
	rand.Shuffle(len(letters), shuffle)
	return string(letters)
}

func getFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}
