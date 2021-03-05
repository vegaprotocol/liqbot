package node

import (
	protobufproto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/vegaprotocol/api-clients/go/generated/code.vegaprotocol.io/vega/proto/api"
)

// Command is a one-byte prefix added to transactions.
type Command byte

// Custom blockchain command encoding, lighter-weight than proto.
const (
	// SubmitOrderCommand ...
	SubmitOrderCommand Command = 0x40
	// CancelOrderCommand ...
	CancelOrderCommand Command = 0x41
	// AmendOrderCommand ...
	AmendOrderCommand Command = 0x42
	// WithdrawCommand ...
	WithdrawCommand Command = 0x44
	// ProposeCommand ...
	ProposeCommand Command = 0x45
	// VoteCommand ...
	VoteCommand Command = 0x46
	// RegisterNodecommand ...
	RegisterNodeCommand Command = 0x47
	// NodeVoteCommand ...
	NodeVoteCommand Command = 0x48
	// NodeSignatureCommand ...
	NodeSignatureCommand Command = 0x49
	// LiquidityProvisionCommand ...
	LiquidityProvisionCommand Command = 0x4A
	// ChainEventCommand ...
	ChainEventCommand Command = 0x50
	// SubmitOracleDataCommand ...
	SubmitOracleDataCommand Command = 0x51
)

// Encode takes a Tx payload and a command and builds a raw Tx.
func Encode(input []byte, cmd Command) ([]byte, error) {
	prefix := uuid.NewV4().String()
	prefixBytes := []byte(prefix)
	commandInput := append([]byte{byte(cmd)}, input...)
	return append(prefixBytes, commandInput...), nil
}

// PrepareLiquidityProvision prepares an API request. Code copied from Core.
func PrepareLiquidityProvision(req *api.PrepareLiquidityProvisionRequest) (*api.PrepareLiquidityProvisionResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(err, "failed to prepare liquidity provision request")
	}

	raw, err := protobufproto.Marshal(req.Submission)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal liquidity provision request")
	}

	if raw, err = Encode(raw, LiquidityProvisionCommand); err != nil {
		return nil, errors.Wrap(err, "failed to encode liquidity provision request")
	}

	return &api.PrepareLiquidityProvisionResponse{Blob: raw}, nil
}
