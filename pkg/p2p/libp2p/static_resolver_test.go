// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"net"
	"runtime"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p/libp2p"
	mockdns "github.com/foxcpp/go-mockdns"
	ma "github.com/multiformats/go-multiaddr"
)

func TestStaticAddressResolver(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skipf("skipped all dns resolver tests on %v", runtime.GOOS)
	}

	for _, tc := range []struct {
		name              string
		natAddr           string
		observableAddress string
		want              string
	}{
		{
			name:              "replace port",
			natAddr:           ":30123",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip4/127.0.0.1/tcp/30123/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v4",
			natAddr:           "192.168.1.34:",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip4/192.168.1.34/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v6",
			natAddr:           "[2001:db8::8a2e:370:1111]:",
			observableAddress: "/ip6/2001:db8::8a2e:370:7334/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip6/2001:db8::8a2e:370:1111/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v4 with ip v6",
			natAddr:           "[2001:db8::8a2e:370:1111]:",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip6/2001:db8::8a2e:370:1111/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v6 with ip v4",
			natAddr:           "192.168.1.34:",
			observableAddress: "/ip6/2001:db8::8a2e:370:7334/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip4/192.168.1.34/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip and port",
			natAddr:           "192.168.1.34:30777",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip4/192.168.1.34/tcp/30777/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v4 and port with ip v6",
			natAddr:           "[2001:db8::8a2e:370:1111]:30777",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip6/2001:db8::8a2e:370:1111/tcp/30777/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v6 and port with ip v4",
			natAddr:           "192.168.1.34:30777",
			observableAddress: "/ip6/2001:db8::8a2e:370:7334/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/ip4/192.168.1.34/tcp/30777/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v6 and port with dns v4",
			natAddr:           "ipv4.com:30777",
			observableAddress: "/ip6/2001:db8::8a2e:370:7334/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/dns4/ipv4.com/tcp/30777/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
		{
			name:              "replace ip v4 and port with dns",
			natAddr:           "ipv4and6.com:30777",
			observableAddress: "/ip4/127.0.0.1/tcp/7071/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
			want:              "/dns/ipv4and6.com/tcp/30777/p2p/16Uiu2HAkyyGKpjBiCkVqCKoJa6RzzZw9Nr7hGogsMPcdad1KyMmd",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := mockdns.NewServer(map[string]mockdns.Zone{
				"ipv4.com.": {
					A: []string{"192.168.1.34"},
				},
				"ipv4and6.com.": {
					A:    []string{"192.168.1.34"},
					AAAA: []string{"2001:db8::8a2e:370:1111"},
				},
			}, false)
			if err != nil {
				t.Fatalf("new mockdns: %v", err)
			}
			defer srv.Close()

			srv.PatchNet(net.DefaultResolver)
			defer mockdns.UnpatchNet(net.DefaultResolver)

			r, err := libp2p.NewStaticAddressResolver(tc.natAddr)
			if err != nil {
				t.Fatal(err)
			}
			observableAddress, err := ma.NewMultiaddr(tc.observableAddress)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Resolve(observableAddress)
			if err != nil {
				t.Fatal(err)
			}

			if got.String() != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}
