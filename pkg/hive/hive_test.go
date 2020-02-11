// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"io/ioutil"
	"testing"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
)

func TestInit(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	hiveProtocol := New(Options{
		Logger: logger,
	})

	t.Run("OK", func(t *testing.T) {
		if err := hiveProtocol.Init(p2p.Peer{}); err != nil {
			t.Fatal("hive init returned an err")
		}
	})
}

func TestService_BroadcastPeers(t *testing.T) {
	logger := logging.New(ioutil.Discard, 0)
	hiveProtocol := New(Options{
		Logger: logger,
	})

	t.Run("OK", func(t *testing.T) {
		if err := hiveProtocol.BroadcastPeers(p2p.Peer{}, []p2p.Peer{}); err != nil {
			t.Fatal("hive init returned an err")
		}
	})
}
