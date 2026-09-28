// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethersphere/bee/v2/pkg/updatecheck"
)

const stageBinary = "binaries/linux-amd64/bee"

var newBinary = []byte("the next bee")

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// stageHarness offers a release listing newBinary and serves served as its
// binary, with pre-staging into a fresh cache.
func stageHarness(t *testing.T, served []byte, cache string) *harness {
	t.Helper()
	rel := release{
		version: 2000, tags: []string{"2.9.0"}, createdAt: time.Now().Add(-48 * time.Hour), window: window(time.Hour),
		files: map[string]string{stageBinary: digestOf(newBinary)},
	}
	h := newHarness(t, rel, func(o *updatecheck.Options) {
		o.Runner.Cache = cache
		o.Runner.Binary = stageBinary
	})
	h.reg.set("/"+stageBinary, response{body: served})
	return h
}

func cacheEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(es))
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestPrestage(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache := t.TempDir()
		h := stageHarness(t, newBinary, cache)
		defer h.s.Close()

		sleep(7*time.Minute + time.Second)
		h.expectShutdowns(t, 1)

		staged := filepath.Join(cache, strings.TrimPrefix(digestOf(newBinary), "sha256:"))
		b, err := os.ReadFile(staged)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, newBinary) {
			t.Fatal("staged binary differs")
		}
		if fi, err := os.Stat(staged); err != nil || fi.Mode().Perm() != 0o755 {
			t.Fatalf("staged binary mode: %v, %v", fi.Mode(), err)
		}
		if got := cacheEntries(t, cache); len(got) != 1 {
			t.Fatalf("cache holds %v, want only the staged binary", got)
		}
		if m := h.s.MetricValues(); m.PrestageDownloads != 1 || m.PrestageErrors != 0 {
			t.Fatalf("metrics: %+v", m)
		}
	})
}

// A binary that does not match the signed digest is never put where the
// runner looks, and the restart goes ahead: the runner downloads it itself.
func TestPrestageDigestMismatch(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache := t.TempDir()
		h := stageHarness(t, []byte("not the signed bee"), cache)
		defer h.s.Close()

		sleep(7*time.Minute + time.Second)
		h.expectShutdowns(t, 1)

		if got := cacheEntries(t, cache); len(got) != 0 {
			t.Fatalf("cache holds %v, want nothing", got)
		}
		if m := h.s.MetricValues(); m.PrestageErrors != 1 || m.PrestageDownloads != 0 {
			t.Fatalf("metrics: %+v", m)
		}
	})
}

func TestPrestageAlreadyCached(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cache := t.TempDir()
		staged := filepath.Join(cache, strings.TrimPrefix(digestOf(newBinary), "sha256:"))
		if err := os.WriteFile(staged, newBinary, 0o755); err != nil {
			t.Fatal(err)
		}
		h := stageHarness(t, newBinary, cache)
		defer h.s.Close()

		sleep(7*time.Minute + time.Second)
		h.expectShutdowns(t, 1)
		if n := h.reg.count("/" + stageBinary); n != 0 {
			t.Fatalf("binary downloaded %d times, want 0", n)
		}
	})
}

// A cache or binary path from the environment that is not what bee-runner
// hands over disables pre-staging, not the restart.
func TestPrestageBadHandoff(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, cache, binary string }{
		{"relative cache", "cache", stageBinary},
		{"unclean cache", "/data/../etc", stageBinary},
		{"binary outside binaries", "/data/bin", "../../etc/passwd"},
		{"binary without platform", "/data/bin", "binaries/bee"},
		{"cache only", "/data/bin", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: time.Now().Add(-48 * time.Hour), window: window(time.Hour),
					files: map[string]string{stageBinary: digestOf(newBinary)}}
				h := newHarness(t, rel, func(o *updatecheck.Options) {
					o.Runner.Cache = tc.cache
					o.Runner.Binary = tc.binary
				})
				defer h.s.Close()
				h.reg.set("/"+stageBinary, response{body: newBinary})

				sleep(7*time.Minute + time.Second)
				h.expectShutdowns(t, 1)
				if n := h.reg.count("/" + stageBinary); n != 0 {
					t.Fatalf("binary downloaded %d times, want 0", n)
				}
			})
		})
	}
}

// A cold download through a gateway only starts once the gateway has
// retrieved the file, far later than the registry's request timeout allows.
func TestPrestageDownloadHeaderTimeout(t *testing.T) {
	t.Parallel()

	if got := updatecheck.DefaultDownloadHeaderTimeout(); got < time.Minute {
		t.Fatalf("download response header timeout %s, want at least a minute", got)
	}
}
