package api

import (
	"context"
	"fmt"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/swarm"
	"io"
)

const MAX_PROTOCOL_SIZE = 1_000_000_000

var PROTOCOLS = map[string]swarm.Address{
	"bzz":    swarm.MustParseHexAddress(""), // TODO: Define this
	"chunks": swarm.MustParseHexAddress(""), // TODO: Define this
}

func (s *server) resolveProtocol(str string) (swarm.Address, error) {
	log := s.logger

	if knownProtocolAddress, exists := PROTOCOLS[str]; exists {
		return knownProtocolAddress, nil
	}

	// Try and parse the name as a bzz address.
	addr, err := swarm.ParseHexAddress(str)
	if err == nil {
		log.Tracef("resolveProtocol: valid bzz address %q", str)
		return addr, nil
	}

	// If no resolver is not available, return an error.
	if s.resolver == nil {
		return swarm.ZeroAddress, errNoResolver
	}

	// Try and resolve the name using the provided resolver.
	log.Debugf("resolveProtocol: attempting to resolve %s to bzz address", str)
	addr, err = s.resolver.Resolve(str)
	if err == nil {
		log.Tracef("resolveProtocol: resolved name %s to %s", str, addr)
		return addr, nil
	}

	return swarm.ZeroAddress, fmt.Errorf("%w: %v", errInvalidNameOrAddress, err)
}

func (s *server) retrieveProtocol(ctx context.Context, reference swarm.Address) ([]byte, error) {
	log := s.logger

	reader, l, err := joiner.New(ctx, s.storer, reference)
	if err != nil {
		return nil, err
	}

	// TODO: Enable this
	//if l > MAX_PROTOCOL_SIZE {
	//	return nil, errors.New("protocol binary too big")
	//}

	protocolBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	log.Tracef("retrieveProtocol: downloaded protocol of size %d", l)

	return protocolBytes, nil
}
