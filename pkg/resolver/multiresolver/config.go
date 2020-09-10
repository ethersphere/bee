// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/ethereum/go-ethereum/common"
)

// Defined as per RFC 1034. For reference, see:
// https://en.wikipedia.org/wiki/Domain_Name_System#cite_note-rfc1034-1
const maxTLDLength = 63

// ConnectionConfig contains the TLD, endpoint and contract address used to
// establish to a resolver.
type ConnectionConfig struct {
	TLD      string
	Address  string
	Endpoint string
}

// ParseConnectionString will try to parse a connection string used to connect
// the Resolver to a name resolution service. The resulting config can be
// used to initialize a resovler Service.
func parseConnectionString(cs string) (ConnectionConfig, error) {
	isAllUnicodeLetters := func(s string) bool {
		for _, r := range s {
			if !unicode.IsLetter(r) {
				return false
			}
		}
		return true
	}

	endpoint := cs
	var tld string
	var addr string

	// Split TLD and Endpoint strings.
	if i := strings.Index(endpoint, ":"); i > 0 {
		// Make sure not to grab the protocol, as it contains "://"!
		// Eg. in http://... the "http" is NOT a tld.
		if isAllUnicodeLetters(endpoint[:i]) && len(endpoint) > i+2 && endpoint[i+1:i+3] != "//" {
			tld = endpoint[:i]
			if len(tld) > maxTLDLength {
				return ConnectionConfig{}, fmt.Errorf("tld %s: %w", tld, ErrTLDTooLong)

			}
			endpoint = endpoint[i+1:]
		}
	}
	// Split the address string.
	if i := strings.Index(endpoint, "@"); i > 0 {
		addr = common.HexToAddress(endpoint[:i]).String()
		endpoint = endpoint[i+1:]
	}

	return ConnectionConfig{
		Endpoint: endpoint,
		Address:  addr,
		TLD:      tld,
	}, nil
}

// ParseConnectionStrings will apply ParseConnectionString to each connection
// string. Returns first error found.
func ParseConnectionStrings(cstrs []string) ([]ConnectionConfig, error) {
	var res []ConnectionConfig

	for _, cs := range cstrs {
		cfg, err := parseConnectionString(cs)
		if err != nil {
			return nil, err
		}
		res = append(res, cfg)
	}

	return res, nil
}
