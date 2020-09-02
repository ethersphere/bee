// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multiresolver_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Address = swarm.Address

func newAddr(s string) Address {
	return swarm.NewAddress([]byte(s))
}

func TestWithForceDefault(t *testing.T) {
	mr := multiresolver.NewMultiResolver(
		multiresolver.WithForceDefault(),
	)

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
		{
			desc:    "invalid tld",
			tld:     "invalid",
			wantErr: multiresolver.ErrInvalidTLD,
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			mr := multiresolver.NewMultiResolver()

			if mr.ChainCount(tC.tld) != 0 {
				t.Fatal("chain should start empty")
			}

			want := mock.NewResolver()
			err := mr.PushResolver(tC.tld, want)
			if err != nil {
				if !errors.Is(err, tC.wantErr) {
					t.Fatal(err)
				}
				return
			}

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
}

func TestPopResolver(t *testing.T) {
	mr := multiresolver.NewMultiResolver()

	t.Run("error on bad tld", func(t *testing.T) {
		if err := mr.PopResolver("invalid"); !errors.Is(err, multiresolver.ErrInvalidTLD) {
			t.Fatal("invalid error type")
		}
	})

	t.Run("error on empty", func(t *testing.T) {
		if err := mr.PopResolver(".tld"); !errors.Is(err, multiresolver.ErrResolverChainEmpty) {
			t.Fatal("invalid error type")
		}
	})
}

func TestResolve(t *testing.T) {
	addr := newAddr("aaaabbbbccccdddd")
	addrAlt := newAddr("ddddccccbbbbaaaa")
	errUnregisteredName := fmt.Errorf("unregistered name")

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
				err := fmt.Errorf("name resolution failed for %q", name)
				return swarm.ZeroAddress, err
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
			wantErr: fmt.Errorf("Failed to resolve name %q", "this.dies"),
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
			if err := mr.PushResolver(tE.tld, r); err != nil {
				t.Fatal(err)
			}
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
