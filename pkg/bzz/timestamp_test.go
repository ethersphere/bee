// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bzz"
)

func TestCheckTimestamp_LegacyUnmarshalledRecord(t *testing.T) {
	t.Parallel()

	// JSON shape produced by pre-timestamp Bee versions: no "timestamp" key.
	legacyJSON := []byte(`{
		"overlay":"0000000000000000000000000000000000000000000000000000000000000001",
		"underlay":"/ip4/127.0.0.1/tcp/1634",
		"underlays":["/ip4/127.0.0.1/tcp/1634"],
		"signature":"",
		"transaction":"02"
	}`)

	var legacy bzz.Address
	if err := legacy.UnmarshalJSON(legacyJSON); err != nil {
		t.Fatalf("UnmarshalJSON of legacy record: %v", err)
	}
	if legacy.Timestamp != 0 {
		t.Fatalf("legacy record Timestamp: got %d, want 0", legacy.Timestamp)
	}

	now := time.Unix(1_700_000_000, 0)

	// Gossip path: legacy existing must be treated as nil (no interval check),
	// and must not panic on the nil-check branch.
	if err := bzz.CheckTimestamp(now.Unix(), &legacy, bzz.TimestampSourceGossip, now); err != nil {
		t.Fatalf("gossip against legacy existing: got %v, want nil", err)
	}

	// Handshake path: same legacy-upgrade behavior.
	if err := bzz.CheckTimestamp(now.Unix(), &legacy, bzz.TimestampSourceHandshake, now); err != nil {
		t.Fatalf("handshake against legacy existing: got %v, want nil", err)
	}

	// Nil existing (ErrNotFound case) must also not panic.
	if err := bzz.CheckTimestamp(now.Unix(), nil, bzz.TimestampSourceGossip, now); err != nil {
		t.Fatalf("gossip against nil existing: got %v, want nil", err)
	}
}

func TestCheckTimestamp(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0)
	interval := int64(bzz.MinimumUpdateInterval.Seconds())
	skew := int64(bzz.MaxClockSkew.Seconds())

	existing := func(ts int64) *bzz.Address {
		return &bzz.Address{Timestamp: ts}
	}

	tests := []struct {
		name     string
		received int64
		existing *bzz.Address
		source   bzz.TimestampSource
		wantErr  error
	}{
		{
			name:     "negative timestamp rejected",
			received: -1,
			existing: nil,
			source:   bzz.TimestampSourceHandshake,
			wantErr:  bzz.ErrTimestampInvalid,
		},
		{
			name:     "zero timestamp rejected on wire",
			received: 0,
			existing: nil,
			source:   bzz.TimestampSourceHandshake,
			wantErr:  bzz.ErrTimestampInvalid,
		},
		{
			name:     "future beyond skew rejected",
			received: now.Unix() + skew + 1,
			existing: nil,
			source:   bzz.TimestampSourceHandshake,
			wantErr:  bzz.ErrTimestampInFuture,
		},
		{
			name:     "future at skew boundary accepted",
			received: now.Unix() + skew,
			existing: nil,
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
		{
			name:     "first-time record accepted",
			received: now.Unix(),
			existing: nil,
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
		{
			name:     "legacy existing (ts=0) upgraded by handshake",
			received: now.Unix(),
			existing: existing(0),
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
		{
			name:     "legacy existing (ts=0) upgraded by gossip",
			received: now.Unix(),
			existing: existing(0),
			source:   bzz.TimestampSourceGossip,
			wantErr:  nil,
		},
		{
			name:     "handshake equal timestamp accepted (re-presentation)",
			received: now.Unix() - 100,
			existing: existing(now.Unix() - 100),
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
		{
			name:     "gossip equal timestamp rejected as stale",
			received: now.Unix() - 100,
			existing: existing(now.Unix() - 100),
			source:   bzz.TimestampSourceGossip,
			wantErr:  bzz.ErrTimestampStale,
		},
		{
			name:     "older timestamp rejected as stale",
			received: now.Unix() - 200,
			existing: existing(now.Unix() - 100),
			source:   bzz.TimestampSourceHandshake,
			wantErr:  bzz.ErrTimestampStale,
		},
		{
			name:     "handshake update with small gap accepted",
			received: now.Unix() - 100 + 1,
			existing: existing(now.Unix() - 100),
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
		{
			name:     "gossip update within interval rejected",
			received: now.Unix() - 1000 + interval - 1,
			existing: existing(now.Unix() - 1000),
			source:   bzz.TimestampSourceGossip,
			wantErr:  bzz.ErrTimestampTooSoon,
		},
		{
			name:     "gossip update at interval boundary rejected",
			received: now.Unix() - 1000 + interval,
			existing: existing(now.Unix() - 1000),
			source:   bzz.TimestampSourceGossip,
			wantErr:  bzz.ErrTimestampTooSoon,
		},
		{
			name:     "gossip update just past interval accepted",
			received: now.Unix() - 1000 + interval + 1,
			existing: existing(now.Unix() - 1000),
			source:   bzz.TimestampSourceGossip,
			wantErr:  nil,
		},
		{
			name:     "handshake update within gossip interval accepted",
			received: now.Unix() - 1000 + 10,
			existing: existing(now.Unix() - 1000),
			source:   bzz.TimestampSourceHandshake,
			wantErr:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := bzz.CheckTimestamp(tc.received, tc.existing, tc.source, now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckTimestamp: got %v, want %v", err, tc.wantErr)
			}
		})
	}
}
