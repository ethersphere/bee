package libp2p

// This error is handled specially by libp2p
// If returned by specific protocol handler it causes peer disconnect
type disconnectError struct {
	err error
}

// Disconnect wraps error and creates a special error that is treated specially by libp2p
// It causes peer disconnect
func Disconnect(err error) error {
	return &disconnectError{
		err: err,
	}
}

// Unwrap returns an underlying error
func (e *disconnectError) Unwrap() error { return e.err }

// Error implements function of the standard go error interface
func (w *disconnectError) Error() string {
	return w.err.Error()
}
