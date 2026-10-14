// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

func TestConstants(t *testing.T) {
	t.Parallel()

	if fingerprint.DefaultMapperID != 1 {
		t.Errorf("DefaultMapperID: got %d, want 1", fingerprint.DefaultMapperID)
	}
	// repeated splits of the root by FanoutBits must end exactly at leaf depth
	if fingerprint.LeafBits%fingerprint.FanoutBits != 0 {
		t.Errorf("LeafBits %d is not a multiple of FanoutBits %d", fingerprint.LeafBits, fingerprint.FanoutBits)
	}
	// the level-1 cache is one split below the root
	if fingerprint.FanoutBits > fingerprint.LeafBits {
		t.Errorf("FanoutBits %d deeper than LeafBits %d", fingerprint.FanoutBits, fingerprint.LeafBits)
	}
}
