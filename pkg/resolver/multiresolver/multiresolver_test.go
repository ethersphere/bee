// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver_test

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Address = swarm.Address

func newAddr(s string) Address {
	return swarm.NewAddress([]byte(s))
}

func TestMultiresolverOpts(t *testing.T) {
	wantLog := logging.New(io.Discard, 1)
	wantCfgs := []multiresolver.ConnectionConfig{
		{
			Address:  "testadr1",
			Endpoint: "testEndpoint1",
			TLD:      "testtld1",
		},
		{
			Address:  "testadr2",
			Endpoint: "testEndpoint2",
			TLD:      "testtld2",
		},
	}

	mr := multiresolver.NewMultiResolver(
		multiresolver.WithLogger(wantLog),
		multiresolver.WithConnectionConfigs(wantCfgs),
		multiresolver.WithForceDefault(),
	)

	if got := multiresolver.GetLogger(mr); got != wantLog {
		t.Errorf("log: got: %v, want %v", got, wantLog)
	}
	if got := multiresolver.GetCfgs(mr); !reflect.DeepEqual(got, wantCfgs) {
		t.Errorf("cfg: got: %v, want %v", got, wantCfgs)
	}
	if !mr.ForceDefault {
		t.Error("did not set ForceDefault")
	}
}

func TestPushResolver(t *testing.T) {
	testCases := []struct {
		desc    string
		tld     string
		wantErr error
	}{
		{
			desc: "empty string, default",
			tld:  "",
		},
		{
			desc: "regular tld, named chain",
			tld:  ".tld",
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			mr := multiresolver.NewMultiResolver()

			if mr.ChainCount(tC.tld) != 0 {
				t.Fatal("chain should start empty")
			}

			want := mock.NewResolver()
			mr.PushResolver(tC.tld, want)

			got := mr.GetChain(tC.tld)[0]
			if !reflect.DeepEqual(got, want) {
				t.Error("failed to push")
			}

			if err := mr.PopResolver(tC.tld); err != nil {
				t.Error(err)
			}
			if mr.ChainCount(tC.tld) > 0 {
				t.Error("failed to pop")
			}
		})
	}
	t.Run("pop empty chain", func(t *testing.T) {
		mr := multiresolver.NewMultiResolver()
		err := mr.PopResolver("")
		if !errors.Is(err, multiresolver.ErrResolverChainEmpty) {
			t.Errorf("got %v, want %v", err, multiresolver.ErrResolverChainEmpty)
		}
	})
}

func TestResolve(t *testing.T) {
	addr := newAddr("aaaabbbbccccdddd")
	addrAlt := newAddr("ddddccccbbbbaaaa")
	errUnregisteredName := fmt.Errorf("unregistered name")
	errResolutionFailed := fmt.Errorf("name resolution failed")

	newOKResolver := func(addr Address) resolver.Interface {
		return mock.NewResolver(
			mock.WithResolveFunc(func(_ string) (Address, error) {
				return addr, nil
			}),
		)
	}
	newErrResolver := func() resolver.Interface {
		return mock.NewResolver(
			mock.WithResolveFunc(func(name string) (Address, error) {
				return swarm.ZeroAddress, errResolutionFailed
			}),
		)
	}
	newUnregisteredNameResolver := func() resolver.Interface {
		return mock.NewResolver(
			mock.WithResolveFunc(func(name string) (Address, error) {
				return swarm.ZeroAddress, errUnregisteredName
			}),
		)
	}

	testFixture := []struct {
		tld       string
		res       []resolver.Interface
		expectAdr Address
	}{
		{
			// Default chain:
			tld: "",
			res: []resolver.Interface{
				newOKResolver(addr),
			},
			expectAdr: addr,
		},
		{
			tld: ".tld",
			res: []resolver.Interface{
				newErrResolver(),
				newErrResolver(),
				newOKResolver(addr),
			},
			expectAdr: addr,
		},
		{
			tld: ".good",
			res: []resolver.Interface{
				newOKResolver(addr),
				newOKResolver(addrAlt),
			},
			expectAdr: addr,
		},
		{
			tld: ".empty",
		},
		{
			tld: ".dies",
			res: []resolver.Interface{
				newErrResolver(),
				newErrResolver(),
			},
		},
		{
			tld: ".unregistered",
			res: []resolver.Interface{
				newUnregisteredNameResolver(),
			},
		},
	}

	testCases := []struct {
		name    string
		wantAdr Address
		wantErr error
	}{
		{
			name:    "",
			wantAdr: addr,
		},
		{
			name:    "hello",
			wantAdr: addr,
		},
		{
			name:    "example.tld",
			wantAdr: addr,
		},
		{
			name:    ".tld",
			wantAdr: addr,
		},
		{
			name:    "get.good",
			wantAdr: addr,
		},
		{
			// Switch to the default chain:
			name:    "this.empty",
			wantAdr: addr,
		},
		{
			name:    "this.dies",
			wantErr: errResolutionFailed,
		},
		{
			name:    "iam.unregistered",
			wantAdr: swarm.ZeroAddress,
			wantErr: errUnregisteredName,
		},
	}

	// Load the test fixture.
	mr := multiresolver.NewMultiResolver()
	for _, tE := range testFixture {
		for _, r := range tE.res {
			mr.PushResolver(tE.tld, r)
		}
	}

	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			addr, err := mr.Resolve(tC.name)
			if err != nil {
				if tC.wantErr == nil {
					t.Fatalf("unexpected error: got %v", err)
				}
				if !errors.Is(err, tC.wantErr) {
					t.Fatalf("got %v, want %v", err, tC.wantErr)
				}
			}
			if !addr.Equal(tC.wantAdr) {
				t.Errorf("got %q, want %q", addr, tC.wantAdr)
			}
		})
	}

	t.Run("close all", func(t *testing.T) {
		if err := mr.Close(); err != nil {
			t.Fatal(err)
		}
		for _, tE := range testFixture {
			for _, r := range mr.GetChain(tE.tld) {
				if !r.(*mock.Resolver).IsClosed {
					t.Errorf("expected %q resolver closed", tE.tld)
				}
			}
		}
	})
}
