// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"strings"

	"github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
)

type UpnpAddressResolver struct {
	host host.Host
}

// Resolve checks if there is a possible better advertisable underlay then the provided observed address.
// In some NAT situations, for example in the case when nodes are behind upnp, observer might send the observed address with a wrong port.
// In this case, observed address is compared to addresses provided by host, and if there is a same address but with different port, that one is used as advertisable address instead of provided observed one.
// TODO: this is a quickfix and it will be improved in the future
func (r *UpnpAddressResolver) Resolve(observedAddress ma.Multiaddr) (ma.Multiaddr, error) {
	observedAddrSplit := strings.Split(observedAddress.String(), "/")

	// if address is not in a form of '/ipversion/ip/protocol/port/...` don't compare to addresses and return it
	if len(observedAddrSplit) < 5 {
		return observedAddress, nil
	}

	observedAddressPort := observedAddrSplit[4]

	// observervedAddressShort is an obaserved address without port
	observervedAddressShort := strings.Join(append(observedAddrSplit[:4], observedAddrSplit[5:]...), "/")

	for _, a := range r.host.Addrs() {
		asplit := strings.Split(a.String(), "/")
		if len(asplit) != len(observedAddrSplit) {
			continue
		}

		aport := asplit[4]
		if strings.Join(append(asplit[:4], asplit[5:]...), "/") != observervedAddressShort {
			continue
		}

		if aport != observedAddressPort {
			return a, nil
		}
	}

	return observedAddress, nil
}
