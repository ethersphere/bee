// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pushsync provides the pushsync protocol
// implementation.
package streamcache_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamcache"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	stream, version, streamName = "test", "0.1", "testStream"
)

func TestStreamIsReused(t *testing.T) {
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	recorder := streamtest.New(streamtest.WithProtocols(protocol(t)), streamtest.WithBaseAddr(pivotNode))

	rc := streamtest.NewRecorderDisconnecter(recorder)

	cache := streamcache.New(rc)

	_, err := cache.NewStream(context.Background(), pivotNode, nil, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cache.NewStream(context.Background(), pivotNode, nil, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}

	recs, err := rc.Records(pivotNode, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}

	if len(recs) != 1 {
		t.Fatal("wanted one record, got", len(recs))
	}
}

func TestStreamIsPurgedOnDisconnect(t *testing.T) {
	pivotNode := swarm.MustParseHexAddress("0000000000000000000000000000000000000000000000000000000000000000") // base is 0000
	recorder := streamtest.New(streamtest.WithProtocols(protocol(t)), streamtest.WithBaseAddr(pivotNode))

	rc := streamtest.NewRecorderDisconnecter(recorder)

	cache := streamcache.New(rc)

	_, err := cache.NewStream(context.Background(), pivotNode, nil, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}

	err = cache.Disconnect(pivotNode, "dummy")
	if err != nil {
		t.Fatal(err)
	}

	_, err = cache.NewStream(context.Background(), pivotNode, nil, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}

	recs, err := rc.Records(pivotNode, stream, version, streamName)
	if err != nil {
		t.Fatal(err)
	}

	if len(recs) != 2 {
		t.Fatal("wanted two records, got", len(recs))
	}
}

func protocol(t *testing.T) p2p.ProtocolSpec {
	t.Helper()
	return p2p.ProtocolSpec{
		Name:    "test",
		Version: "0.1",
		StreamSpecs: []p2p.StreamSpec{
			{
				Name: "testStream",
				Handler: func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
					return nil
				},
			},
		},
	}
}
