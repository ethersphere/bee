// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package feeds_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/feeds"
)

func TestTypeFromString(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"sequence", "Sequence"} {
		var typ feeds.Type
		if err := typ.FromString(s); err != nil {
			t.Fatalf("%q: unexpected error: %v", s, err)
		}
		if typ != feeds.Sequence {
			t.Fatalf("%q: got type %v, want %v", s, typ, feeds.Sequence)
		}
	}

	// epoch feeds are not supported
	for _, s := range []string{"epoch", "Epoch", "", "unknown"} {
		var typ feeds.Type
		if err := typ.FromString(s); !errors.Is(err, feeds.ErrFeedTypeNotFound) {
			t.Fatalf("%q: got error %v, want %v", s, err, feeds.ErrFeedTypeNotFound)
		}
	}
}
