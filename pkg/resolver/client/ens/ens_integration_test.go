// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build integration

package ens_test

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver/client/ens"
)

func TestENSntegration(t *testing.T) {
	// TODO: consider using a stable gateway instead of INFURA.
	defaultEndpoint := "https://goerli.infura.io/v3/59d83a5a4be74f86b9851190c802297b"

	testCases := []struct {
		desc            string
		endpoint        string
		name            string
		wantAdr         string
		wantFailConnect bool
		wantFailResolve bool
	}{
		{
			desc:            "bad ethclient endpoint",
			endpoint:        "fail",
			wantFailConnect: true,
		},
		{
			desc:            "no domain",
			name:            "idonthaveadomain",
			wantFailResolve: true,
		},
		{
			desc:            "no eth domain",
			name:            "centralized.com",
			wantFailResolve: true,
		},
		{
			desc:            "not registered",
			name:            "unused.test.swarm.eth",
			wantFailResolve: true,
		},
		{
			desc:            "no content hash",
			name:            "nocontent.resolver.test.swarm.eth",
			wantFailResolve: true,
		},
		{
			desc:    "ok",
			name:    "example.resolver.test.swarm.eth",
			wantAdr: "00cb23598c2e520b6a6aae3ddc94fed4435a2909690bdd709bf9d9e7c2aadfad",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.endpoint == "" {
				tC.endpoint = defaultEndpoint
			}

			eC := ens.NewClient()

			err := eC.Connect(tC.endpoint)
			if err != nil {
				if !tC.wantFailConnect {
					t.Fatalf("failed to connect: %v", err)
				}
				return
			}

			adr, err := eC.Resolve(tC.name)
			if err != nil {
				if !tC.wantFailResolve {
					t.Fatalf("failed to resolve name: %v", err)
				}
				return
			}

			want := strings.ToLower(tC.wantAdr)
			got := strings.ToLower(adr.String())
			if got != want {
				t.Errorf("bad addr: got %q, want %q", got, want)
			}

			eC.Close()
			if eC.IsConnected() {
				t.Errorf("IsConnected: got true, want false")
			}
		})
	}
}
