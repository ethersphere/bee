// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package p2p_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
)

func newTestProtocol(h p2p.HandlerFunc) p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    "test",
		Version: "1.0",
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    "testStreamName1",
				Handler: h,
			}, {
				Name:    "testStreamName2",
				Handler: h,
			},
		},
	}
}

func TestBlocklistError(t *testing.T) {
	tp := newTestProtocol(func(context.Context, p2p.Peer, p2p.Stream) error {
		return errors.New("test")
	})

	p2p.WithBlocklistStreams(1*time.Minute, tp)

	for _, sp := range tp.StreamSpecs {
		err := sp.Handler(context.Background(), p2p.Peer{}, nil)
		var discErr *p2p.BlockPeerError
		if !errors.As(err, &discErr) {
			t.Error("unexpected error type")
		}
		if !errors.Is(err, p2p.ErrUnexpected) {
			t.Error("unexpected wrapped error type")
		}
	}
}

func TestDisconnectError(t *testing.T) {
	tp := newTestProtocol(func(context.Context, p2p.Peer, p2p.Stream) error {
		return errors.New("test")
	})

	p2p.WithDisconnectStreams(tp)

	for _, sp := range tp.StreamSpecs {
		err := sp.Handler(context.Background(), p2p.Peer{}, nil)
		var discErr *p2p.DisconnectError
		if !errors.As(err, &discErr) {
			t.Error("unexpected error type")
		}
		if !errors.Is(err, p2p.ErrUnexpected) {
			t.Error("unexpected wrapped error type")
		}
	}
}
