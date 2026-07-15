// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package addressbook

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/storage"
)

type VerifiedAddress = verifiedAddress

// NewWithClock creates an addressbook with an overridable clock, for testing.
func NewWithClock(storer storage.StateStorer, now func() time.Time) Interface {
	return newStore(storer, now)
}
