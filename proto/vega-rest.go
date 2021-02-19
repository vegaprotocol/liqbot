package proto

import (
	"strconv"

	p "github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto"
)

// RESTPriceLevel is a RESTified PriceLevel.
type RESTPriceLevel struct {
	Price          string `json:"price"`          // uint64
	NumberOfOrders string `json:"numberOfOrders"` // uint64
	Volume         string `json:"volume"`         // uint64
}

// RESTSignedBundle is a RESTified SignedBundle.
type RESTSignedBundle struct {
	Tx  []byte         `json:"tx,omitempty"`
	Sig *RESTSignature `json:"sig,omitempty"`
}

// RESTSignature is a RESTified Signature.
type RESTSignature struct {
	Sig     []byte `json:"sig,omitempty"`
	Algo    string `json:"algo,omitempty"`
	Version uint64 `json:"version,omitempty"`
}

// ConvertPriceLevel converts a RESTPriceLevel into a PriceLevel
func ConvertPriceLevel(lvl *RESTPriceLevel) (*p.PriceLevel, error) {
	if lvl == nil {
		return nil, nil
	}
	price, err := strconv.ParseUint(lvl.Price, 10, 64)
	if err != nil {
		return nil, err
	}
	numOrders, err := strconv.ParseUint(lvl.NumberOfOrders, 10, 64)
	if err != nil {
		return nil, err
	}
	volume, err := strconv.ParseUint(lvl.Volume, 10, 64)
	if err != nil {
		return nil, err
	}
	return &p.PriceLevel{
		Price:          uint64(price),
		NumberOfOrders: uint64(numOrders),
		Volume:         uint64(volume),
	}, nil
}
