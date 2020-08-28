// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package service

import (
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/resolver/client/ens"
)

// InitMultiResolver will create a new MultiResolver, create the appropriate
// resolvers, push them to the resolver chains and attempt to connect.
func InitMultiResolver(logger logging.Logger, cfgs []*resolver.ConnectionConfig) *resolver.MultiResolver {
	if len(cfgs) > 0 {
		logger.Info("name resolver: no name resolution service provided")
	}

	// Create a new MultiResolver.
	mr := resolver.NewMultiResolver()

	connectENS := func(tld string, ep string) {
		ensCl := ens.NewClient()

		logger.Debugf("name resolver: resolver for %q: connecting to endpoint %q")
		if err := ensCl.Connect(ep); err != nil {
			logger.Errorf("name resolver: resolver for %q domain: failed to connect to %q: %v", tld, ep, err)
		} else {
			logger.Infof("name resolver: resolver for %q domain: connected to %q", tld, ep)
			if err := mr.PushResolver(tld, ens.NewClient()); err != nil {
				logger.Errorf("name resolver: failed to push resolver to %q resolver chain: %v", tld, err)
			}
		}
	}

	// Atrempt to conect to each resolver using the connection string.
	for _, c := range cfgs {

		// Warn user that the resolver address field is not used.
		if c.Address != "" {
			logger.Warningf("name resolver: connection string %q contains resolver address field, which is currently unused", c.Address)
		}

		logger.Debugf("name resolver: attempting connection at %q", c)

		// Select the appropriate resolver.
		switch c.TLD {
		case "eth":
			// FIXME: MultiResolver expects "." in front of the TLD label.
			connectENS("."+c.TLD, c.Endpoint)
		case "":
			connectENS("", c.Endpoint)
		default:
			logger.Errorf("default domain resolution not supported")
		}
	}

	return mr
}
