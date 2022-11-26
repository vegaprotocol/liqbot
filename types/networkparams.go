package types

import (
	"time"

	"code.vegaprotocol.io/vega/core/netparams"
)

type NetworkParameters struct {
	Params map[string]string
}

func NewNetworkParameters(params map[string]string) (*NetworkParameters, error) {
	return &NetworkParameters{
		Params: params,
	}, nil
}

type MarketProposalParams struct {
	MinClose time.Duration
	MinEnact time.Duration
}

func (n *NetworkParameters) GetMarketProposalParams() (*MarketProposalParams, error) {
	minClose, err := time.ParseDuration(n.Params[netparams.GovernanceProposalMarketMinClose])
	if err != nil {
		return nil, err
	}
	minEnact, err := time.ParseDuration(n.Params[netparams.GovernanceProposalMarketMinEnact])
	if err != nil {
		return nil, err
	}
	return &MarketProposalParams{
		MinClose: minClose,
		MinEnact: minEnact,
	}, nil
}
