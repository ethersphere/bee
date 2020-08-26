// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/resolver/client/ens"
)

// Service is the name resolution service ready for integration with bee.
type Service struct {
	cfgs   []*ConnectionConfig
	multi  *resolver.MultiResolver
	logger logging.Logger
}

// ConnectionConfig contains the TLD, endpoint and contract address used to
// establish to a resolver.
type ConnectionConfig struct {
	TLD      string
	Address  string
	Endpoint string
}

// NewService creates a new Service with the given options.
func NewService(cfgs []*ConnectionConfig, logger logging.Logger) *Service {
	return &Service{
		cfgs:   cfgs,
		logger: logger,
		multi:  resolver.NewMultiResolver(),
	}
}

// Connect will attempt to connect all resolvers their configured endpoints.
func (s *Service) Connect() {
	log := s.logger

	connectENS := func(tld string, ep string) {
		ensCl := ens.NewClient()
		if err := ensCl.Connect(ep); err != nil {
			log.Errorf("name resolver for %q domain failed to connect to %q: %v", tld, ep, err)
		} else {
			log.Infof("name resolver for %q domain connected to %q", tld, ep)
			if err := s.multi.PushResolver(tld, ens.NewClient()); err != nil {
				log.Errorf("failed to push name resolver to %q resolver chain: %v", tld, err)
			}
		}
	}

	for _, c := range s.cfgs {

		// Warn user that the resolver address field is not used.
		if c.Address != "" {
			log.Warningf("connection string %q contains resolver address field, which is currently unused", c.Address)
		}

		// Select the appropriate resolver.
		switch c.TLD {
		case "eth":
			// TODO: MultiResolver expect "." in front of the TLD label.
			connectENS("."+c.TLD, c.Endpoint)
		case "":
			connectENS("", c.Endpoint)
		default:
			log.Errorf("default domain resolution not supported")
		}
	}
}

// Close implements the Closer interface.
func (s *Service) Close() error {
	s.multi.Close()
	return nil
}

// ParseConnectionString will try to parse a connection string used to connect
// the Resolver to a name resolution service. The resulting config can be
// used to initialize a resovler Service.
func ParseConnectionString(cs string) (*ConnectionConfig, error) {
	isAllUnicodeLetters := func(s string) bool {
		for _, r := range s {
			if !unicode.IsLetter(r) {
				return false
			}
		}
		return true
	}

	// Defined as per RFC 1034. For reference, see:
	// https://en.wikipedia.org/wiki/Domain_Name_System#cite_note-rfc1034-1
	const maxLabelLength = 63

	endpoint := cs
	var tld string
	var adr string

	// Split TLD and Endpoint strings.
	if i := strings.Index(endpoint, ":"); i > 0 {
		// Make sure not to grab the protocol, as it contains "://"!
		// Eg. in http://... the "http" is NOT a tld.
		if isAllUnicodeLetters(endpoint[:i]) && len(endpoint) > i+2 && endpoint[i+1:i+3] != "//" {
			tld = endpoint[:i]
			if len(tld) > maxLabelLength {
				return nil, fmt.Errorf("Resolver connection string: TLD extend max length of %d characters", maxLabelLength)

			}
			endpoint = endpoint[i+1:]
		}
	}
	// Split the address string.
	if i := strings.Index(endpoint, "@"); i > 0 {
		adr = common.HexToAddress(endpoint[:i]).String()
		endpoint = endpoint[i+1:]
	}

	return &ConnectionConfig{
		Endpoint: endpoint,
		Address:  adr,
		TLD:      tld,
	}, nil
}

// ParseConnectionStrings will apply ParseConnectionString to each connection
// string. Returns first error found.
func ParseConnectionStrings(cstrs []string) ([]*ConnectionConfig, error) {
	var res []*ConnectionConfig

	for _, cs := range cstrs {
		cfg, err := ParseConnectionString(cs)
		if err != nil {
			return nil, err
		}
		res = append(res, cfg)
	}

	return res, nil
}
