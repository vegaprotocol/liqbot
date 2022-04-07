package errors

import "errors"

var (
	// ErrNil indicates that a null pointer was encountered. This should never happen.
	ErrNil = errors.New("null pointer")

	// ErrInterrupted indicates that a sleep was interrupted.
	ErrInterrupted = errors.New("interrupted")

	// ErrConnectionNotReady indicated that the network connection to the gRPC server is not ready.
	ErrConnectionNotReady = errors.New("gRPC connection not ready")
)
