package api

import (
	"strconv"

	proto1 "code.vegaprotocol.io/liqbot/proto"

	p "code.vegaprotocol.io/vega/proto"
	pa "code.vegaprotocol.io/vega/proto/api"
)

// RESTMarketDepthResponse is a RESTified MarketDepthResponse.
type RESTMarketDepthResponse struct {
	MarketID string                   `json:"marketId"`
	Buy      []*proto1.RESTPriceLevel `json:"buy"`
	Sell     []*proto1.RESTPriceLevel `json:"sell"`
	// LastTrade *proto1.RESTTrade        `json:"lastTrade"`
}

// RESTMarketsResponse is a RESTified MarketsResponse.
type RESTMarketsResponse struct {
	Markets []*proto1.RESTMarket `json:"markets"`
}

// RESTPrepareSubmitOrderResponse is a RESTified PrepareSubmitOrderResponse.
type RESTPrepareSubmitOrderResponse struct {
	Blob     []byte `json:"blob,omitempty"`
	SubmitID string `json:"submitId,omitempty"`
}

// RESTSubmitTransactionRequest is a RESTified SubmitTransactionRequest.
type RESTSubmitTransactionRequest struct {
	Tx *proto1.RESTSignedBundle `json:"tx,omitempty"`
}

// RESTGetVegaTimeResponse is a RESTified GetVegaTimeResponse.
type RESTGetVegaTimeResponse struct {
	Timestamp string `json:"timestamp"` // int64
}

// ConvertMarketDepthResponse converts a RESTMarketDepthResponse into a MarketDepthResponse
func ConvertMarketDepthResponse(r *RESTMarketDepthResponse) (*pa.MarketDepthResponse, error) {
	if r == nil {
		return nil, nil
	}

	buy := make([]*p.PriceLevel, len(r.Buy))
	for i, lvl := range r.Buy {
		l, err := proto1.ConvertPriceLevel(lvl)
		if err != nil {
			return nil, err
		}
		buy[i] = l
	}

	sell := make([]*p.PriceLevel, len(r.Sell))
	for i, lvl := range r.Sell {
		l, err := proto1.ConvertPriceLevel(lvl)
		if err != nil {
			return nil, err
		}
		sell[i] = l
	}
	return &pa.MarketDepthResponse{
		MarketId: r.MarketID,
		Buy:      buy,
		Sell:     sell,
	}, nil
}

// ConvertSubmitTransactionRequest converts a SubmitTransactionRequest into a RESTSubmitTransactionRequest.
func ConvertSubmitTransactionRequest(r *pa.SubmitTransactionRequest) (*RESTSubmitTransactionRequest, error) {
	if r == nil {
		return nil, nil
	}
	req := &RESTSubmitTransactionRequest{
		Tx: &proto1.RESTSignedBundle{
			Tx: r.Tx.Tx,
			Sig: &proto1.RESTSignature{
				Sig:     r.Tx.Sig.Sig,
				Algo:    r.Tx.Sig.Algo,
				Version: r.Tx.Sig.Version,
			},
		},
	}
	return req, nil
}

// ConvertGetVegaTimeResponse converts a RESTGetVegaTimeResponse into a GetVegaTimeResponse
func ConvertGetVegaTimeResponse(r *RESTGetVegaTimeResponse) (*pa.GetVegaTimeResponse, error) {
	if r == nil {
		return nil, nil
	}
	nsec, err := strconv.ParseInt(r.Timestamp, 10, 64)
	if err != nil {
		return nil, err
	}
	return &pa.GetVegaTimeResponse{
		Timestamp: nsec,
	}, nil
}
