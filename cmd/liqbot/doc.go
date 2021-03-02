// Command liqbot runs one application instance which may contain many bots that
// provide liquidity. Each bot connects to one Vega node and submits orders to
// one market.
//
//     $ make install
//     $ $GOPATH/bin/liqbot -config=config.yml
package main
