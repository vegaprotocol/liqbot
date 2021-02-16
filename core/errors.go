package core

import "errors"

// Errors
var (
	ErrRequestBodyReadFail    = errors.New("failed to read request body")
	ErrEmptyReference         = errors.New("empty reference")
	ErrEmptyResponse          = errors.New("empty response")
	ErrEmptyToken             = errors.New("empty token")
	ErrFailedToGetVegaTime    = errors.New("failed to get Vega time")
	ErrFailedToGetMarkets     = errors.New("failed to get list of markets")
	ErrInsaneDate             = errors.New("insane date")
	ErrInterrupted            = errors.New("interrupted")
	ErrMarketNotFound         = errors.New("market not found")
	ErrNegativeTime           = errors.New("negative time")
	ErrNil                    = errors.New("nil pointer")
	ErrNodeNotFound           = errors.New("node not found")
	ErrNoMarketDepth          = errors.New("no market depth")
	ErrNoMarkets              = errors.New("no markets")
	ErrNotConnected           = errors.New("not connected")
	ErrPartyNotFound          = errors.New("party not found")
	ErrTraderAlreadyActive    = errors.New("trader already active")
	ErrTraderAlreadyStopped   = errors.New("trader already stopped")
	ErrTraderNotFound         = errors.New("trader not found")
	ErrUnrecognisedAction     = errors.New("unrecognised action")
	ErrUnsupportedScheme      = errors.New("unsupported scheme")
	ErrServerResponseNone     = errors.New("no response from server")
	ErrServerResponseEmpty    = errors.New("empty response from server")
	ErrServerResponseReadFail = errors.New("failed to read response from server")
	ErrTraderAlreayExists     = errors.New("trader already exists")
)
