// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/pushsync"
)

// The switches of the simulator are kept in this file, so that the rest of
// the simulator compiles against a commit that does not have them.

func simSetNoHopMemory(_ *testing.T, ps *pushsync.PushSync, v bool) { ps.SetNoHopMemory(v) }

func simSetLegacyForwarder(_ *testing.T, ps *pushsync.PushSync, v bool) { ps.SetLegacyForwarder(v) }
