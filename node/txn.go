package node

import (
	e "code.vegaprotocol.io/liqbot/errors"

	protobufproto "github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/vegaprotocol/api/go/generated/code.vegaprotocol.io/vega/proto/api"
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

const (
	failedValidate = "failed to validate request"
)

// MarshalEncode takes a Tx payload and a command and builds a raw Tx.
func MarshalEncode(submission protobufproto.Message, cmd Command) (encoded []byte, err error) {
	raw, err := protobufproto.Marshal(submission)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}

	prefixBytes := []byte(uuid.NewV4().String())
	commandInput := append([]byte{byte(cmd)}, raw...)
	return append(prefixBytes, commandInput...), nil
}

// PrepareLiquidityProvision prepares an API request. Code copied from Core.
func PrepareLiquidityProvision(req *api.PrepareLiquidityProvisionRequest) (*api.PrepareLiquidityProvisionResponse, error) {
	if req == nil || req.Submission == nil {
		return nil, e.ErrNil
	}
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(err, failedValidate)
	}

	raw, err := MarshalEncode(req.Submission, LiquidityProvisionCommand)
	if err != nil {
		return nil, err // no need to wrap
	}

	return &api.PrepareLiquidityProvisionResponse{Blob: raw}, nil
}

// PrepareSubmitOrder prepares an API request. Code copied from Core.
func PrepareSubmitOrder(req *api.PrepareSubmitOrderRequest) (*api.PrepareSubmitOrderResponse, error) {
	if req == nil || req.Submission == nil {
		return nil, e.ErrNil
	}
	if err := req.Validate(); err != nil {
		return nil, errors.Wrap(err, failedValidate)
	}

	if req.Submission.Reference == "" {
		req.Submission.Reference = uuid.NewV4().String()
	}
	raw, err := MarshalEncode(req.Submission, SubmitOrderCommand)
	if err != nil {
		return nil, err // no need to wrap
	}
	return &api.PrepareSubmitOrderResponse{
		Blob:     raw,
		SubmitId: req.Submission.Reference,
	}, nil
}
