// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"strings"

	"github.com/coreos/go-semver/semver"
	protocol "github.com/libp2p/go-libp2p-core/protocol"
)

// protocolSemverMatcher returns a matcher function for a given base protocol.
// Protocol ID must be constructed according to the Swarm protocol ID
// specification, where the second to last part is the version is semver format.
// The matcher function will return a boolean indicating whether a protocol ID
// matches the base protocol. A given protocol ID matches the base protocol if
// the IDs are the same and if the semantic version of the base protocol is the
// same or higher than that of the protocol ID provided.
func (s *Service) protocolSemverMatcher(base protocol.ID) (func(string) bool, error) {
	parts := strings.Split(string(base), "/")
	partsLen := len(parts)
	if partsLen < 2 {
		return nil, errors.New("invalid protocol id")
	}
	vers, err := semver.NewVersion(parts[partsLen-2])
	if err != nil {
		return nil, err
	}

	return func(check string) bool {
		chparts := strings.Split(check, "/")
		chpartsLen := len(chparts)
		if chpartsLen != partsLen {
			return false
		}

		for i, v := range chparts {
			if i == chpartsLen-2 {
				continue
			}
			if parts[i] != v {
				return false
			}
		}

		chvers, err := semver.NewVersion(chparts[chpartsLen-2])
		if err != nil {
			s.logger.Debugf("invalid protocol version %q: %v", check, err)
			return false
		}

		return vers.Major == chvers.Major && vers.Minor >= chvers.Minor
	}, nil
}
