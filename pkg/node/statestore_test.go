// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
)

// TestStateStoreDirPermissions verifies that the statestore and stamperstore
// directories are not readable by other users on the host. goleveldb creates
// them with mode 0o755 on its own, so they are pre-created by the node.
func TestStateStoreDirPermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on windows")
	}

	dataDir := t.TempDir()

	stateStore, _, err := node.InitStateStore(log.Noop, dataDir, 1024)
	if err != nil {
		t.Fatalf("InitStateStore: %v", err)
	}
	t.Cleanup(func() {
		if err := stateStore.Close(); err != nil {
			t.Errorf("close state store: %v", err)
		}
	})

	stamperStore, _, err := node.InitStamperStore(log.Noop, dataDir, stateStore)
	if err != nil {
		t.Fatalf("InitStamperStore: %v", err)
	}
	t.Cleanup(func() {
		if err := stamperStore.Close(); err != nil {
			t.Errorf("close stamper store: %v", err)
		}
	})

	for _, name := range []string{"statestore", "stamperstore"} {
		path := filepath.Join(dataDir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := fi.Mode().Perm(); got != 0o700 {
			t.Errorf("%s: got mode %O, want %O", path, got, 0o700)
		}
	}
}
