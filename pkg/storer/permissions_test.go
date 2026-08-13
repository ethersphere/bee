// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestDiskStorePermissions verifies that the directories and shard files a disk
// based storer creates are not readable by other users on the host.
func TestDiskStorePermissions(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on windows")
	}

	basePath := t.TempDir()

	st, err := storer.New(context.Background(), basePath, dbTestOps(swarm.RandAddress(t), 1000, nil, nil, time.Minute))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	assertMode(t, filepath.Join(basePath, "indexstore"), 0o700)
	assertMode(t, filepath.Join(basePath, "sharky"), 0o700)

	entries, err := os.ReadDir(filepath.Join(basePath, "sharky"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no sharky shard files created")
	}
	for _, e := range entries {
		assertMode(t, filepath.Join(basePath, "sharky", e.Name()), 0o600)
	}
}

func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s: got mode %O, want %O", path, got, want)
	}
}
