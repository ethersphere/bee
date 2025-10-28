// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
)

func TestNewBee_InvalidNATAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		natAddr string
	}{
		{
			name:    "empty host",
			natAddr: ":1635",
		},
		{
			name:    "localhost",
			natAddr: "localhost:1635",
		},
		{
			name:    "loopback",
			natAddr: "127.0.0.1:1635",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			privateKey, err := crypto.GenerateSecp256k1Key()
			if err != nil {
				t.Fatal(err)
			}

			opts := &node.Options{
				NATAddr: tc.natAddr,
			}

			_, err = node.NewBee(context.Background(), "", &privateKey.PublicKey, nil, 0, log.Noop, privateKey, privateKey, nil, opts)
			if err == nil {
				t.Fatal("expected error, but got none")
			}
		})
	}
}
