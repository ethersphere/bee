// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/bee/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewClient(t *testing.T) {
	cl := ens.NewClient()
	if cl.Endpoint != "" {
		t.Errorf("expected no endpoint set")
	}
}

func TestConnect(t *testing.T) {
	ep := "test"

	t.Run("no dial func error", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithDialFunc(nil),
		)
		err := c.Connect(ep)
		defer c.Close()
		if !errors.Is(err, ens.ErrNotImplemented) {
			t.Fatal("expected correct error")
		}
	})

	t.Run("connect error", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithErrorDialFunc(errors.New("failed to connect")),
		)

		if err := c.Connect("test"); err == nil {
			t.Fatal("expected error")
		}
		c.Close()
	})

	t.Run("ok", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithNoopDialFunc(),
		)

		if err := c.Connect(ep); err != nil {
			t.Fatal(err)
		}
		// Override the eth client to test connection.
		ens.SetEthClient(c, &ethclient.Client{})

		if c.Endpoint != ep {
			t.Errorf("bad endpoint: got %q, want %q", c.Endpoint, ep)
		}

		if !c.IsConnected() {
			t.Error("IsConnected: got false, want true")
		}

		// We are not really connected, so clear the client to prevent panic.
		ens.SetEthClient(c, nil)
		c.Close()
		if c.IsConnected() {
			t.Error("IsConnected: got true, want false")
		}

	})
}

func TestResolve(t *testing.T) {
	name := "hello"
	bzzAddress := swarm.MustParseHexAddress(
		"6f4eeb99d0a144d78ac33cf97091a59a6291aa78929938defcf967e74326e08b",
	)

	t.Run("no resolve func error", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithResolveFunc(nil),
		)
		_, err := c.Resolve("test")
		if !errors.Is(err, ens.ErrNotImplemented) {
			t.Fatal("expected correct error")
		}
	})

	t.Run("resolve error", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithNoopDialFunc(),
			ens.WithErrorResolveFunc(errors.New("resolve error")),
		)

		if err := c.Connect(name); err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		_, err := c.Resolve(name)
		if !errors.Is(err, ens.ErrResolveFailed) {
			t.Error("expected resolve error")
		}
	})

	t.Run("zero address returned", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithNoopDialFunc(),
			ens.WithZeroAdrResolveFunc(),
		)

		if err := c.Connect(name); err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		_, err := c.Resolve(name)
		if !errors.Is(err, ens.ErrNameNotFound) {
			t.Error("expected name not found error")
		}
	})

	t.Run("resolved without address prefix error", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithNoopDialFunc(),
			ens.WithNoprefixAdrResolveFunc(bzzAddress),
		)

		if err := c.Connect(name); err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		_, err := c.Resolve(name)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("ok", func(t *testing.T) {
		c := ens.NewClient(
			ens.WithNoopDialFunc(),
			ens.WithValidAdrResolveFunc(bzzAddress),
		)

		if err := c.Connect(name); err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		addr, err := c.Resolve(name)
		if err != nil {
			t.Error(err)
		}
		want := bzzAddress.String()
		got := addr.String()
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

}
