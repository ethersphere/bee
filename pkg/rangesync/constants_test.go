// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
)

func TestConstants(t *testing.T) {
	t.Parallel()

	if rangesync.StartupGuardTimeout != 2*rangesync.StatusMaxInterval {
		t.Errorf("StartupGuardTimeout %s is not 2 x StatusMaxInterval %s", rangesync.StartupGuardTimeout, rangesync.StatusMaxInterval)
	}
	if rangesync.SyncRateWindow != puller.DefaultHistRateWindow {
		t.Errorf("SyncRateWindow %s differs from the puller's %s", rangesync.SyncRateWindow, puller.DefaultHistRateWindow)
	}
	if rangesync.LeafDepthSlack != fingerprint.FanoutBits {
		t.Errorf("LeafDepthSlack %d is not one split level (%d)", rangesync.LeafDepthSlack, fingerprint.FanoutBits)
	}
	if rangesync.StatusMinInterval >= rangesync.StatusMaxInterval {
		t.Errorf("StatusMinInterval %s not below StatusMaxInterval %s", rangesync.StatusMinInterval, rangesync.StatusMaxInterval)
	}
	if rangesync.MinRoundInterval >= rangesync.MaxRoundInterval {
		t.Errorf("MinRoundInterval %s not below MaxRoundInterval %s", rangesync.MinRoundInterval, rangesync.MaxRoundInterval)
	}
	if rangesync.MaxMessageBytes != 1<<20 {
		t.Errorf("MaxMessageBytes: got %d, want 1 MiB", rangesync.MaxMessageBytes)
	}
}
