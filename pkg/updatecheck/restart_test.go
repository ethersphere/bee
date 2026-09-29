// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/updatecheck"
)

var overlay = swarm.MustParseHexAddress("ca1e9f3938cc1425c6061b96ad9eb93e134dfe8734ad490164ef20af9d1cf59c")

// harness runs the service with the update restart active, inside a synctest
// bubble.
type harness struct {
	reg       *registry
	s         *updatecheck.Service
	dir       string
	shutdowns atomic.Int32
}

func newHarness(t *testing.T, rel release, opts ...func(*updatecheck.Options)) *harness {
	t.Helper()
	h := &harness{reg: newRegistry(), dir: t.TempDir()}
	h.reg.offer(t, rel)
	o := runnerOptions(h.reg)
	o.Overlay = overlay
	o.Restart = updatecheck.RestartOptions{
		Enabled:  true,
		DataDir:  h.dir,
		Shutdown: func() { h.shutdowns.Add(1) },
	}
	for _, f := range opts {
		f(&o)
	}
	s, err := updatecheck.New(log.Noop, o)
	if err != nil {
		t.Fatal(err)
	}
	if !s.RestartActive() {
		t.Fatal("restart inactive")
	}
	h.s = s
	return h
}

// sleep advances the bubble's clock by d and lets the service settle.
func sleep(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

func (h *harness) expectShutdowns(t *testing.T, want int32) {
	t.Helper()
	if got := h.shutdowns.Load(); got != want {
		t.Fatalf("shutdowns: got %d, want %d", got, want)
	}
}

func (h *harness) marker(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, updatecheck.MarkerFileName))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func writeMarker(t *testing.T, dir string, targetVersion uint64, attempts int, at time.Time) {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"targetVersion": targetVersion, "target": "2.9.0", "attempts": attempts, "at": at.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, updatecheck.MarkerFileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRestartSlot(t *testing.T) {
	t.Parallel()

	base := time.Unix(1_700_000_000, 0)
	w := 24 * time.Hour
	slot := updatecheck.RestartSlot(overlay.Bytes(), 2000, base, w)
	if slot.Before(base) || !slot.Before(base.Add(w)) {
		t.Fatalf("slot %s outside the window", slot)
	}
	if again := updatecheck.RestartSlot(overlay.Bytes(), 2000, base, w); !again.Equal(slot) {
		t.Fatal("slot is not deterministic")
	}
	if other := updatecheck.RestartSlot(overlay.Bytes(), 2001, base, w); other.Equal(slot) {
		t.Fatal("slot does not depend on the release")
	}
	if got := updatecheck.RestartSlot(overlay.Bytes(), 2000, base, 0); !got.Equal(base) {
		t.Fatalf("zero window: got %s, want %s", got, base)
	}

	for _, tc := range []struct{ window, want time.Duration }{
		{0, time.Minute},
		{20 * time.Minute, 2 * time.Minute},
		{24 * time.Hour, time.Hour},
	} {
		if got := updatecheck.LateJitter(tc.window); got != tc.want {
			t.Errorf("late jitter for %s: got %s, want %s", tc.window, got, tc.want)
		}
	}
}

func TestRestartAtSlot(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		w := 30 * 24 * time.Hour
		// Place the slot about 3h from now. createdAt has whole seconds.
		offset := updatecheck.RestartSlot(overlay.Bytes(), 2000, start, w).Sub(start)
		createdAt := start.Add(3*time.Hour - offset).Truncate(time.Second)
		if createdAt.After(start) {
			t.Skip("slot too early in the window for this overlay")
		}
		slot := updatecheck.RestartSlot(overlay.Bytes(), 2000, createdAt, w)
		rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: createdAt, window: window(w)}
		h := newHarness(t, rel)
		defer h.s.Close()

		sleep(time.Until(slot) - time.Second)
		h.expectShutdowns(t, 0)
		if got := h.s.MetricValues().RestartScheduled; got != float64(slot.Unix()) {
			t.Fatalf("restart scheduled at %v, want %v", got, slot.Unix())
		}

		sleep(2 * time.Second)
		h.expectShutdowns(t, 1)
		m := h.marker(t)
		if m["targetVersion"] != float64(2000) || m["attempts"] != float64(1) || m["target"] != "2.9.0" {
			t.Fatalf("marker %v", m)
		}
	})
}

// A node that notices a release after its slot restarts after a random delay.
// The delay is bounded by the window.
func TestRestartLateNode(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: start.Add(-48 * time.Hour), window: window(time.Hour)}
		h := newHarness(t, rel)
		defer h.s.Close()

		sleep(time.Minute)
		at := h.s.MetricValues().RestartScheduled
		if at == 0 || at > float64(start.Add(7*time.Minute).Unix()) {
			t.Fatalf("restart scheduled at %v", at)
		}
		sleep(6*time.Minute + time.Second)
		h.expectShutdowns(t, 1)
	})
}

func TestRestartWaitsForSafePoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		unsafeFor  int
		wantBefore time.Duration
	}{
		{"safe point reached", 3, 4 * time.Minute},
		{"never safe", -1, 23 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				var calls atomic.Int32
				gate := func() (bool, string) {
					n := int(calls.Add(1))
					return tc.unsafeFor >= 0 && n > tc.unsafeFor, "claim phase"
				}
				rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: start.Add(-time.Hour), window: window(0)}
				h := newHarness(t, rel, func(o *updatecheck.Options) {
					o.Restart.Gate = gate
					o.Restart.RoundDuration = 10 * time.Minute
				})
				defer h.s.Close()

				if tc.unsafeFor < 0 {
					// Wait for two rounds, counted from the restart time (at
					// most 2m in).
					sleep(20 * time.Minute)
					h.expectShutdowns(t, 0)
				}
				sleep(tc.wantBefore - time.Since(start))
				h.expectShutdowns(t, 1)
				if tc.unsafeFor >= 0 && calls.Load() != int32(tc.unsafeFor+1) {
					t.Fatalf("gate called %d times", calls.Load())
				}
			})
		})
	}
}

// A restart for a release that was not delivered is retried once after a
// backoff. While the restart is pending, the registry is polled more often.
// Once the release is withdrawn, the restart is canceled and the configured
// interval applies again.
func TestRestartRetryAndPolling(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: start.Add(-48 * time.Hour), window: window(20 * time.Minute)}
		dir := t.TempDir()
		// bee restarted for 2000 a moment ago, but it runs 1000 again.
		writeMarker(t, dir, 2000, 1, start)
		h := newHarness(t, rel, func(o *updatecheck.Options) {
			o.Interval = time.Hour
			o.Restart.DataDir = dir
		})
		h.dir = dir
		defer h.s.Close()

		// The retry waits for the backoff and polls every quarter window.
		sleep(updatecheck.RetryBackoff - time.Minute)
		h.expectShutdowns(t, 0)
		if got, want := h.s.MetricValues().RestartScheduled, float64(start.Add(updatecheck.RetryBackoff).Unix()); got != want {
			t.Fatalf("restart scheduled at %v, want %v", got, want)
		}
		times := h.reg.requestTimes("/release.json")
		if len(times) < 5 {
			t.Fatalf("%d checks while a restart is pending", len(times))
		}
		for i := 1; i < len(times); i++ {
			if gap := times[i].Sub(times[i-1]); gap > 6*time.Minute {
				t.Fatalf("check %d after %s while a restart is pending", i, gap)
			}
		}

		// The release is withdrawn before the restart fires, so the restart is
		// canceled.
		h.reg.offer(t, release{version: 900, tags: []string{"2.8.0"}})
		sleep(2 * time.Minute)
		h.expectShutdowns(t, 0)
		if got := h.s.MetricValues().RestartScheduled; got != 0 {
			t.Fatalf("restart still scheduled at %v", got)
		}
		n := h.reg.count("/release.json")
		sleep(50 * time.Minute)
		if got := h.reg.count("/release.json"); got > n+1 {
			t.Fatalf("%d checks in 50m after the restart was canceled", got-n)
		}

		// The release is offered again. This is the second and last attempt.
		h.reg.offer(t, rel)
		sleep(time.Hour + 3*time.Minute)
		h.expectShutdowns(t, 1)
		if m := h.marker(t); m["attempts"] != float64(2) {
			t.Fatalf("marker %v", m)
		}
	})
}

func TestRestartSuppressed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		marker bool
		opts   func(*updatecheck.Options)
	}{
		{name: "previous restarts did not deliver", marker: true},
		{name: "bee-runner rolled back", opts: func(o *updatecheck.Options) { o.Runner.RolledBack = "2000" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: start.Add(-48 * time.Hour), window: window(0)}
				dir := t.TempDir()
				if tc.marker {
					writeMarker(t, dir, 2000, 2, start.Add(-2*time.Hour))
				}
				h := newHarness(t, rel, func(o *updatecheck.Options) {
					o.Restart.DataDir = dir
					if tc.opts != nil {
						tc.opts(o)
					}
				})
				defer h.s.Close()

				sleep(2 * time.Hour)
				h.expectShutdowns(t, 0)
				if got := h.s.MetricValues(); got.RestartSuppressed != 1 || got.Available != 1 || got.RestartScheduled != 0 {
					t.Fatalf("metrics %+v", got)
				}

				// A newer release lifts the suppression.
				h.reg.offer(t, release{version: 2001, tags: []string{"2.9.1"}, createdAt: start, window: window(0)})
				sleep(time.Hour)
				h.expectShutdowns(t, 1)
				if got := h.s.MetricValues().RestartSuppressed; got != 0 {
					t.Fatalf("still suppressed")
				}
			})
		})
	}
}

func TestRestartMarkerAtStartup(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: start.Add(-48 * time.Hour), window: window(0)}

		// Delivered: the marker is removed.
		dir := t.TempDir()
		writeMarker(t, dir, 1000, 1, start)
		h := newHarness(t, rel, func(o *updatecheck.Options) { o.Restart.DataDir = dir })
		if _, err := os.Stat(filepath.Join(dir, updatecheck.MarkerFileName)); !os.IsNotExist(err) {
			t.Fatalf("marker kept: %v", err)
		}
		h.s.Close()

		// A symbolic link is not followed, and nothing is written through it.
		dir = t.TempDir()
		target := filepath.Join(t.TempDir(), "elsewhere")
		if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, updatecheck.MarkerFileName)); err != nil {
			t.Fatal(err)
		}
		h = newHarness(t, rel, func(o *updatecheck.Options) { o.Restart.DataDir = dir })
		h.dir = dir
		defer h.s.Close()
		sleep(5 * time.Minute)
		h.expectShutdowns(t, 1)
		if b, err := os.ReadFile(target); err != nil || string(b) != "keep" {
			t.Fatalf("link target changed: %q, %v", b, err)
		}
		if fi, err := os.Lstat(filepath.Join(dir, updatecheck.MarkerFileName)); err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("marker is not a regular file: %v", err)
		}
		if m := h.marker(t); m["targetVersion"] != float64(2000) {
			t.Fatalf("marker %v", m)
		}
	})
}

func TestRestartCloseWhilePending(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		rel := release{version: 2000, tags: []string{"2.9.0"}, createdAt: time.Now(), window: window(30 * 24 * time.Hour)}
		h := newHarness(t, rel)
		sleep(time.Hour)
		if h.s.MetricValues().RestartScheduled == 0 {
			t.Fatal("no restart scheduled")
		}
		if err := h.s.Close(); err != nil {
			t.Fatal(err)
		}
		h.expectShutdowns(t, 0)
	})
}

func TestRestartActivation(t *testing.T) {
	t.Parallel()

	enabled := func(f func(*updatecheck.Options)) updatecheck.Options {
		o := runnerOptions(newRegistry())
		o.URL = ""
		o.Restart = updatecheck.RestartOptions{Enabled: true, DataDir: t.TempDir(), Shutdown: func() {}}
		f(&o)
		return o
	}

	s, err := updatecheck.NewUnstarted(log.Noop, enabled(func(*updatecheck.Options) {}))
	if err != nil {
		t.Fatal(err)
	}
	if !s.RestartActive() || s.Registry() != registryURL {
		t.Fatalf("active %v, registry %q", s.RestartActive(), s.Registry())
	}

	for _, tc := range []struct {
		name string
		f    func(*updatecheck.Options)
	}{
		{"not started by bee-runner", func(o *updatecheck.Options) { o.Runner.Started = false }},
		{"no release version", func(o *updatecheck.Options) { o.Runner.Version = "" }},
		{"invalid release version", func(o *updatecheck.Options) { o.Runner.Version = "v1" }},
		{"no release key", func(o *updatecheck.Options) { o.Runner.Pubkey = "" }},
		{"no data directory", func(o *updatecheck.Options) { o.Restart.DataDir = "" }},
		{"unnamed rollback", func(o *updatecheck.Options) { o.Runner.RolledBack = "yes" }},
	} {
		// With the URL configured, the check still runs, but only reports.
		o := enabled(tc.f)
		o.URL = registryURL
		s, err := updatecheck.NewUnstarted(log.Noop, o)
		if err != nil {
			t.Fatal(err)
		}
		if s.RestartActive() {
			t.Errorf("%s: restart active", tc.name)
		}
		// Without the URL, an inactive restart does not enable the check.
		if s, err := updatecheck.NewUnstarted(log.Noop, enabled(tc.f)); s != nil || err != nil {
			t.Errorf("%s: check enabled without a url: %v", tc.name, err)
		}
	}
}
