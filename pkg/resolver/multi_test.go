// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package resolver_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/resolver/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Address = swarm.Address

func newAddr(s string) Address {
	return swarm.NewAddress([]byte(s))
}

func TestWithForceDefault(t *testing.T) {
	mr := resolver.NewMultiResolver(
		resolver.WithForceDefault(),
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
			wantErr: resolver.ErrInvalidTLD("invalid"),
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			mr := resolver.NewMultiResolver()

			if mr.ChainCount(tC.tld) != 0 {
				t.Fatal("chain should start empty")
			}

			want := mock.NewResolver()
			err := mr.PushResolver(tC.tld, want)
			if err != nil {
				if errors.Is(err, tC.wantErr) {
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
	mr := resolver.NewMultiResolver()

	t.Run("error on bad tld", func(t *testing.T) {
		err := mr.PopResolver("invalid")
		want := resolver.ErrInvalidTLD("invalid")
		if err != want {
			t.Fatalf("bad error: got %v, want %v", err, want)
		}
	})

	t.Run("error on empty", func(t *testing.T) {
		err := mr.PopResolver(".tld")
		want := resolver.ErrResolverChainEmpty(".tld")
		if err != want {
			t.Fatalf("bad error: got %v, want %v", err, want)
		}
	})
}

func TestResolve(t *testing.T) {
	testAdr := newAddr("aaaabbbbccccdddd")
	testAdrAlt := newAddr("ddddccccbbbbaaaa")

	newOKResolver := func(adr Address) resolver.Interface {
		return mock.NewResolver(
			mock.WithResolveFunc(func(_ string) (Address, error) {
				return adr, nil
			}),
		)
	}
	newErrResolver := func() resolver.Interface {
		return mock.NewResolver(
			mock.WithResolveFunc(func(name string) (Address, error) {
				err := fmt.Errorf("name resolution failed for %q", name)
				return Address{}, err
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
				newOKResolver(testAdr),
			},
			expectAdr: testAdr,
		},
		{
			tld: ".tld",
			res: []resolver.Interface{
				newErrResolver(),
				newErrResolver(),
				newOKResolver(testAdr),
			},
			expectAdr: testAdr,
		},
		{
			tld: ".good",
			res: []resolver.Interface{
				newOKResolver(testAdr),
				newOKResolver(testAdrAlt),
			},
			expectAdr: testAdr,
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
	}

	testCases := []struct {
		name    string
		wantAdr Address
		wantErr error
	}{
		{
			name:    "",
			wantAdr: testAdr,
		},
		{
			name:    "hello",
			wantAdr: testAdr,
		},
		{
			name:    "example.tld",
			wantAdr: testAdr,
		},
		{
			name:    ".tld",
			wantAdr: testAdr,
		},
		{
			name:    "get.good",
			wantAdr: testAdr,
		},
		{
			name:    "this.empty",
			wantErr: resolver.ErrResolverChainEmpty(".empty"),
		},
		{
			name:    "this.dies",
			wantErr: fmt.Errorf("Failed to resolve name %q", "this.dies"),
		},
	}

	// Load the test fixture.
	mr := resolver.NewMultiResolver()
	for _, tE := range testFixture {
		for _, r := range tE.res {
			if err := mr.PushResolver(tE.tld, r); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, tC := range testCases {
		t.Run(tC.name, func(t *testing.T) {
			adr, err := mr.Resolve(tC.name)
			if err != nil {
				if tC.wantErr == nil {
					t.Fatalf("unexpected error: got %v", err)
				}
				if !errors.Is(err, tC.wantErr) {
					t.Fatalf("got %v, want %v", err, tC.wantErr)
				}
			}
			if !adr.Equal(tC.wantAdr) {
				t.Errorf("got %q, want %q", adr, tC.wantAdr)
			}
		})
	}

	t.Run("close all", func(t *testing.T) {
		mr.Close()
		for _, tE := range testFixture {
			for _, r := range mr.GetChain(tE.tld) {
				if !r.(*mock.Resolver).IsClosed {
					t.Errorf("expected %q resolver closed", tE.tld)
				}
			}
		}
	})
}
