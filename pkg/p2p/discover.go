// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func isDNSProtocol(protoCode int) bool {
	if protoCode == ma.P_DNS || protoCode == ma.P_DNS4 || protoCode == ma.P_DNS6 || protoCode == ma.P_DNSADDR {
		return true
	}
	return false
}

func Discover(ctx context.Context, addr ma.Multiaddr, f func(ma.Multiaddr) (bool, error)) (bool, error) {
	if comp, _ := ma.SplitFirst(addr); !isDNSProtocol(comp.Protocol().Code) {
		return f(addr)
	}

	dnsResolver := madns.DefaultResolver
	addrs, err := dnsResolver.Resolve(ctx, addr)
	if err != nil {
		return false, fmt.Errorf("dns resolve address %s: %w", addr, err)
	}
	if len(addrs) == 0 {
		return false, errors.New("non-resolvable API endpoint")
	}

	// sort addrs so that quic is tried first
	sort.Slice(addrs, func(i, j int) bool {
		return strings.Contains(addrs[i].String(), "/quic")
	})

	for _, addr := range addrs {
		stopped, err := Discover(ctx, addr, f)
		if err != nil {
			return false, fmt.Errorf("discover %s: %w", addr, err)
		}

		if stopped {
			return true, nil
		}
	}

	return false, nil
}
