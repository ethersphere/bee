package trojan

import (
	"errors"
	"fmt"
)

var (
	// ErrPayloadTooBig is returned when a given payload for a Message type is longer than the maximum amount allowed
	ErrPayloadTooBig = fmt.Errorf("message payload size cannot be greater than %d bytes", MaxPayloadSize)

	// ErrEmptyTargets is returned when the given target list for a trojan chunk is empty
	ErrEmptyTargets = errors.New("target list cannot be empty")

	// ErrVarLenTargets is returned when the given target list for a trojan chunk has addresses of different lengths
	ErrVarLenTargets = errors.New("target list cannot have targets of different length")

	// ErrUnMarshallingTrojanMessage is returned when a trojan message could not be de-serialized
	ErrUnmarshal = errors.New("trojan message unmarshall error")

	// ErrMinerTimeout is returned when mining a new nonce takes more time than swarm.TrojanMinerTimeout seconds
	ErrMinerTimeout = errors.New("miner timeout error")
)
