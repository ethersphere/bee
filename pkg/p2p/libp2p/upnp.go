// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"errors"
	"strings"

	"github.com/libp2p/go-libp2p/core/host"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// UpnpAddressResolver intelligently handles NAT scenarios by:
// 1. First trying to use a matching listen address (preferred)
// 2. If no match, using the observed address (NAT-aware)
// 3. Providing logging for debugging NAT issues
type UpnpAddressResolver struct {
	host host.Host
}

// NewUpnpAddressResolver creates a new resolver that handles NAT scenarios intelligently.
func NewUpnpAddressResolver(host host.Host) *UpnpAddressResolver {
	return &UpnpAddressResolver{
		host: host,
	}
}

// Resolve intelligently resolves the advertised address considering NAT scenarios.
func (r *UpnpAddressResolver) Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error) {
	observableAddrInfo, err := libp2ppeer.AddrInfoFromP2pAddr(observedAddress)
	if err != nil {
		return nil, err
	}

	if len(observableAddrInfo.Addrs) < 1 {
		return nil, errors.New("invalid observed address")
	}

	observedAddrSplit := strings.Split(observableAddrInfo.Addrs[0].String(), "/")

	// if address is not in a form of '/ipversion/ip/protocol/port/...` don't compare to addresses and return it
	if len(observedAddrSplit) < 5 {
		return observedAddress, nil
	}

	observedAddressPort := observedAddrSplit[4]
	observervedAddressShort := strings.Join(append(observedAddrSplit[:4], observedAddrSplit[5:]...), "/")

	// Strategy 1: Try to find a matching listen address with the same port
	// This is the ideal case - external port matches internal port
	for _, a := range r.host.Addrs() {
		asplit := strings.Split(a.String(), "/")
		if len(asplit) != len(observedAddrSplit) {
			continue
		}

		aport := asplit[4]
		if strings.Join(append(asplit[:4], asplit[5:]...), "/") != observervedAddressShort {
			continue
		}

		// Found a matching address with the same port - this is ideal
		if aport == observedAddressPort {
			aaddress, err := buildUnderlayAddress(a, observableAddrInfo.ID)
			if err != nil {
				continue
			}
			return aaddress, nil
		}
	}

	// Strategy 2: Try to find any matching listen address (different port)
	// This handles cases where NAT remaps ports but we want to use our listen port
	for _, a := range r.host.Addrs() {
		asplit := strings.Split(a.String(), "/")
		if len(asplit) != len(observedAddrSplit) {
			continue
		}

		if strings.Join(append(asplit[:4], asplit[5:]...), "/") != observervedAddressShort {
			continue
		}

		// Found a matching address but with different port
		// This might work if the NAT is configured to forward the observed port to our listen port
		aaddress, err := buildUnderlayAddress(a, observableAddrInfo.ID)
		if err != nil {
			continue
		}
		return aaddress, nil
	}

	// Strategy 3: Fall back to observed address
	// This handles complex NAT scenarios where the observed address is the most reliable
	return observedAddress, nil
}
