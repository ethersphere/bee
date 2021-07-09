// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build integration

package ens_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestENSIntegration(t *testing.T) {
	// TODO: consider using a stable gateway instead of INFURA.
	defaultEndpoint := "https://goerli.infura.io/v3/59d83a5a4be74f86b9851190c802297b"
	defaultAddr := swarm.MustParseHexAddress("00cb23598c2e520b6a6aae3ddc94fed4435a2909690bdd709bf9d9e7c2aadfad")

	testCases := []struct {
		desc            string
		endpoint        string
		contractAddress string
		name            string
		wantAdr         swarm.Address
		wantErr         error
	}{
		// TODO: add a test targeting a resolver with an invalid contenthash
		// record.
		{
			desc:     "invalid resolver endpoint",
			endpoint: "example.com",
			wantErr:  ens.ErrFailedToConnect,
		},
		{
			desc:    "no domain",
			name:    "idonthaveadomain",
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc:    "no eth domain",
			name:    "centralized.com",
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc:    "not registered",
			name:    "unused.test.swarm.eth",
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc:    "no content hash",
			name:    "nocontent.resolver.test.swarm.eth",
			wantErr: ens.ErrResolveFailed,
		},
		{
			desc:            "invalid contract address",
			contractAddress: "0xFFFFFFFF",
			name:            "example.resolver.test.swarm.eth",
			wantErr:         ens.ErrFailedToConnect,
		},
		{
			desc:    "ok",
			name:    "example.resolver.test.swarm.eth",
			wantAdr: defaultAddr,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			if tC.endpoint == "" {
				tC.endpoint = defaultEndpoint
			}

			ensClient, err := ens.NewClient(tC.endpoint, ens.WithContractAddress(tC.contractAddress))
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got %v, want %v", err, tC.wantErr)
				}
				return
			}
			defer ensClient.Close()

			addr, err := ensClient.Resolve(tC.name)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Errorf("got %v, want %v", err, tC.wantErr)
				}
				return
			}

			if !addr.Equal(defaultAddr) {
				t.Errorf("bad addr: got %s, want %s", addr, defaultAddr)
			}

			err = ensClient.Close()
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
