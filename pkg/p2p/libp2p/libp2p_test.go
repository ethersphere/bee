// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"context"
	"errors"
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/libp2p"
)

func TestAddresses(t *testing.T) {
	s := newService(t, &libp2p.Options{
		NetworkID: 1,
	})

	addrs, err := s.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	if l := len(addrs); l == 0 {
		t.Fatal("no addresses")
	}
}

func TestConnectDisconnect(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}
}

func TestDoubleConnect(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	if _, err := s2.Connect(context.Background(), addr); err != nil {
		t.Fatal(err)
	}

	if _, err := s2.Connect(context.Background(), addr); err == nil {
		t.Fatal("second connect attempt should result with an error")
	}
}

func TestDoubleDisconnect(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}
	if err := s2.Disconnect(overlay); !errors.Is(err, p2p.ErrPeerNotFound) {
		t.Errorf("got error %v, want %v", err, p2p.ErrPeerNotFound)
	}
}

func TestReconnectAfterDoubleConnect(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s2.Connect(context.Background(), addr); err == nil {
		t.Fatal("second connect attempt should result with an error")
	}

	overlay, err = s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if !overlay.Equal(o1.Overlay) {
		t.Errorf("got overlay %s, want %s", overlay, o1.Overlay)
	}
}

func TestMultipleConnectDisconnect(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	addr := addrs[0]

	overlay, err := s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}

	overlay, err = s2.Connect(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}

	if err := s2.Disconnect(overlay); err != nil {
		t.Fatal(err)
	}
}

func TestConnectDisconnectOnAllAddresses(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		overlay, err := s2.Connect(context.Background(), addr)
		if err != nil {
			t.Fatal(err)
		}

		if err := s2.Disconnect(overlay); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDoubleConnectOnAllAddresses(t *testing.T) {
	o1 := libp2p.Options{
		NetworkID: 1,
	}
	s1 := newService(t, &o1)
	o2 := libp2p.Options{
		NetworkID: 1,
	}
	s2 := newService(t, &o2)

	addrs, err := s1.Addresses()
	if err != nil {
		t.Fatal(err)
	}
	for _, addr := range addrs {
		if _, err := s2.Connect(context.Background(), addr); err != nil {
			t.Fatal(err)
		}
		if _, err := s2.Connect(context.Background(), addr); err == nil {
			t.Fatal("second connect attempt should result with an error")
		}
	}
}

func newService(t *testing.T, o *libp2p.Options) *libp2p.Service {
	t.Helper()

	if o == nil {
		o = new(libp2p.Options)
	}

	if o.PrivateKey == nil {
		var err error
		o.PrivateKey, err = crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
	}

	if o.Overlay.IsZero() {
		var err error
		swarmPK, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		o.Overlay = crypto.NewAddress(swarmPK.PublicKey)
	}

	if o.Logger == nil {
		o.Logger = logging.New(ioutil.Discard, 0)
	}

	if o.Addr == "" {
		o.Addr = ":0"
	}

	s, err := libp2p.New(context.Background(), *o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
