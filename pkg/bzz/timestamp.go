// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bzz

import (
	"errors"
	"time"
)

type TimestampSource int

const (
	TimestampSourceHandshake TimestampSource = iota
	TimestampSourceGossip
)

const (
	MaxClockSkew          = 60 * time.Second
	MinimumUpdateInterval = 300 * time.Second
)

var (
	ErrTimestampInvalid  = errors.New("bzz: timestamp must be positive")
	ErrTimestampInFuture = errors.New("bzz: timestamp in future")
	ErrTimestampStale    = errors.New("bzz: timestamp not newer than existing")
	ErrTimestampTooSoon  = errors.New("bzz: timestamp within minimum update interval")
)

// TimestampErrorLabel returns the metric label for a CheckTimestamp error,
// or ("", false) if err is not a timestamp sentinel.
func TimestampErrorLabel(err error) (string, bool) {
	switch {
	case errors.Is(err, ErrTimestampInvalid):
		return "invalid", true
	case errors.Is(err, ErrTimestampInFuture):
		return "in_future", true
	case errors.Is(err, ErrTimestampStale):
		return "stale", true
	case errors.Is(err, ErrTimestampTooSoon):
		return "too_soon", true
	}
	return "", false
}

// CheckTimestamp validates a newly received record's timestamp against an
// existing stored record. existing is nil when no prior record is held for
// the overlay. Semantics follow issue #6:
//
//   - wire records must carry a strictly-positive Timestamp (zero and
//     negative values are rejected);
//   - records with a timestamp more than MaxClockSkew in the future are
//     rejected;
//   - first-time records are accepted;
//   - an existing stored record with Timestamp == 0 (legacy, pre-timestamp)
//     is overwritten by any received record (which is guaranteed > 0);
//   - handshake records are rejected as stale only when strictly older than
//     the existing record. Equal timestamps are accepted (re-presentation of
//     the peer's current record during reconnect is benign);
//   - gossip records must be strictly newer than existing, and additionally
//     must exceed the existing Timestamp by at least MinimumUpdateInterval.
func CheckTimestamp(newTimestamp int64, existing *Address, source TimestampSource, now time.Time) error {
	if newTimestamp <= 0 {
		return ErrTimestampInvalid
	}

	if newTimestamp > now.Add(MaxClockSkew).Unix() {
		return ErrTimestampInFuture
	}

	if existing == nil || existing.Timestamp == 0 {
		return nil
	}

	switch source {
	case TimestampSourceHandshake:
		if newTimestamp < existing.Timestamp {
			return ErrTimestampStale
		}
	case TimestampSourceGossip:
		if newTimestamp <= existing.Timestamp {
			return ErrTimestampStale
		}
		if newTimestamp <= existing.Timestamp+int64(MinimumUpdateInterval.Seconds()) {
			return ErrTimestampTooSoon
		}
	}

	return nil
}
