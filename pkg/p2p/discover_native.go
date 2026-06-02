//go:build !js

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	madns "github.com/multiformats/go-multiaddr-dns"
)

// newDNSResolver returns the default system DNS resolver for native builds.
func newDNSResolver() (*madns.Resolver, error) {
	return madns.DefaultResolver, nil
}
