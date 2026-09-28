// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/updatecheck"
)

func TestCheck(t *testing.T) {
	t.Parallel()

	outside := func(reg *registry) updatecheck.Options {
		return updatecheck.Options{URL: registryURL, CurrentVersion: "2.8.0-7e703f49", Client: reg.client()}
	}
	// Under bee-runner, but without a release key: report-only from /info.
	unsigned := func(reg *registry) updatecheck.Options {
		o := runnerOptions(reg)
		o.Runner.Pubkey = ""
		return o
	}

	for _, tc := range []struct {
		name     string
		opts     func(*registry) updatecheck.Options
		current  string
		info     []byte
		rel      release
		want     updatecheck.Result
		wantErr  error
		verified bool
	}{
		{
			name: "newer tag", opts: outside, verified: true,
			rel:  release{version: 2000, tags: []string{"2.7.0", "v2.9.0", "2.9.1-rc1", "latest"}},
			want: updatecheck.Result{Current: "2.8.0", Latest: "2.9.0", LatestVersion: 2000, Available: true},
		},
		{
			name: "same tag", opts: outside, verified: true,
			rel:  release{version: 2000, tags: []string{"2.8.0"}},
			want: updatecheck.Result{Current: "2.8.0", Latest: "2.8.0", LatestVersion: 2000},
		},
		{
			name: "final release after the running candidate", opts: outside, verified: true, current: "2.8.0-rc3-7e703f49-dirty",
			rel:  release{version: 2000, tags: []string{"2.8.0"}},
			want: updatecheck.Result{Current: "2.8.0-rc.3", Latest: "2.8.0", LatestVersion: 2000, Available: true},
		},
		{
			name: "only pre-release tags", opts: outside, verified: true,
			rel:     release{version: 2000, tags: []string{"2.9.0-rc1"}},
			wantErr: updatecheck.ErrNoReleaseTags,
		},
		{
			name: "unparsable own version", opts: outside, verified: true, current: "devel",
			rel:     release{version: 2000, tags: []string{"2.9.0"}},
			wantErr: updatecheck.ErrUnparsableVersion,
		},
		{
			name: "unverified info", opts: outside,
			rel:     release{version: 2000, tags: []string{"2.9.0"}},
			wantErr: updatecheck.ErrUnverified,
		},
		{
			name: "runner: newer descriptor version, same tag", opts: unsigned, verified: true,
			rel:  release{version: 2000, tags: []string{"2.8.0"}},
			want: updatecheck.Result{Current: "2.8.0", Latest: "2.8.0", LatestVersion: 2000, Available: true},
		},
		{
			name: "runner: older descriptor version, higher tag", opts: unsigned, verified: true,
			rel:  release{version: 900, tags: []string{"2.9.0"}},
			want: updatecheck.Result{Current: "2.8.0", Latest: "2.9.0", LatestVersion: 900},
		},
		{
			name: "runner: newer release on another channel", opts: unsigned, verified: true,
			rel:  release{version: 2000, channels: []string{"beta"}, tags: []string{"2.9.0"}},
			want: updatecheck.Result{Current: "2.8.0", Latest: "2.9.0", LatestVersion: 2000, OtherChannel: true},
		},
		{
			name: "runner: no descriptor version", opts: unsigned, verified: true,
			rel:     release{tags: []string{"2.9.0"}},
			wantErr: updatecheck.ErrNoReleaseVersion,
		},
		{
			name: "runner: own version need not parse", opts: unsigned, verified: true, current: "devel",
			rel:  release{version: 2000},
			want: updatecheck.Result{Current: "devel", LatestVersion: 2000, Available: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := newRegistry()
			reg.set("/info", response{body: info(t, tc.rel, tc.verified)})
			o := tc.opts(reg)
			if tc.current != "" {
				o.CurrentVersion = tc.current
			}
			s, err := updatecheck.NewUnstarted(log.Noop, o)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.Check(context.Background())
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error: got %v, want %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if reg.count("/release.json") != 0 {
				t.Fatal("fetched the signed descriptor without a release key")
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"2.8.2-7e703f49", "2.8.2", 0},
		{"v2.8.2", "2.8.2-7e703f49-dirty", 0},
		{"2.8.10", "2.8.9", 1},
		{"2.9.0-rc1-7e703f49", "2.9.0", -1},
		{"2.9.0-rc2", "2.9.0-rc10", -1},
		{"2.9.0-rc.2", "2.9.0-rc2", 0},
		{"3.0.0-rc1", "2.9.9", 1},
	} {
		got, ok := updatecheck.CompareVersions(tc.a, tc.b)
		if !ok || got != tc.want {
			t.Errorf("compare(%q, %q): got %d (%v), want %d", tc.a, tc.b, got, ok, tc.want)
		}
	}
	for _, v := range []string{"", "devel", "2.9", "2.9.0-rc0", "x2.9.0"} {
		if _, ok := updatecheck.CompareVersions(v, v); ok {
			t.Errorf("parsed %q", v)
		}
	}
}

// Registry responses end up in error messages and logs: neither the status
// line nor the body of a response is repeated, and neither are credentials in
// the registry URL.
func TestUntrustedDataNotLogged(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		reg := newRegistry()
		reg.set("/info", response{status: http.StatusBadGateway, body: []byte("body from the registry")})
		logger, out := logs()

		o := updatecheck.Options{URL: "https://user:secret@registry.example/", CurrentVersion: "2.8.0", Client: reg.client()}
		s, err := updatecheck.New(logger, o)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		time.Sleep(time.Minute + time.Second)
		synctest.Wait()
		if reg.count("/info") != 1 {
			t.Fatalf("got %d checks, want 1", reg.count("/info"))
		}
		if got := s.MetricValues().CheckErrors; got != 1 {
			t.Fatalf("check errors: got %v, want 1", got)
		}
		logged := out.String()
		if !strings.Contains(logged, "502 Bad Gateway") {
			t.Fatalf("failed check not logged with its status code:\n%s", logged)
		}
		for _, leak := range []string{"secret", "status from the registry", "body from the registry"} {
			if strings.Contains(logged, leak) {
				t.Fatalf("logged %q:\n%s", leak, logged)
			}
		}
		if strings.Contains(logged, `"level"="warning"`) || strings.Contains(logged, `"level"="error"`) {
			t.Fatalf("failed check logged above debug:\n%s", logged)
		}
	})
}

func TestResponseLimits(t *testing.T) {
	t.Parallel()

	reg := newRegistry()
	g := updatecheck.NewGetter(reg.client())
	ctx := context.Background()

	reg.set("/doc", response{body: bytes.Repeat([]byte{'a'}, 99)})
	if _, err := g.Get(ctx, registryURL+"/doc", 100); err != nil {
		t.Fatalf("under the limit: %v", err)
	}
	// Refused at the limit, as bee-runner refuses it.
	reg.set("/doc", response{body: bytes.Repeat([]byte{'a'}, 100)})
	if _, err := g.Get(ctx, registryURL+"/doc", 100); !errors.Is(err, updatecheck.ErrResponseTooLarge) {
		t.Fatalf("at the limit: got %v, want %v", err, updatecheck.ErrResponseTooLarge)
	}
}

func TestConditionalGet(t *testing.T) {
	t.Parallel()

	reg := newRegistry()
	g := updatecheck.NewGetter(reg.client())
	ctx := context.Background()

	reg.set("/doc", response{body: []byte("v1"), etag: `"1"`})
	for i := range 2 {
		b, err := g.Get(ctx, registryURL+"/doc", 100)
		if err != nil || string(b) != "v1" {
			t.Fatalf("get %d: got %q, %v", i, b, err)
		}
	}
	if got := reg.lastRequest("/doc").Header.Get("If-None-Match"); got != `"1"` {
		t.Fatalf("If-None-Match: got %q", got)
	}

	reg.set("/doc", response{body: []byte("v2")})
	if b, err := g.Get(ctx, registryURL+"/doc", 100); err != nil || string(b) != "v2" {
		t.Fatalf("changed: got %q, %v", b, err)
	}
	if _, err := g.Get(ctx, registryURL+"/doc", 100); err != nil {
		t.Fatal(err)
	}
	if got := reg.lastRequest("/doc").Header.Get("If-None-Match"); got != "" {
		t.Fatalf("revalidated a response without an entity tag: %q", got)
	}
}

func TestDefaultClient(t *testing.T) {
	t.Parallel()

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("other origin"))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, "/doc", http.StatusFound)
		case "/cross":
			http.Redirect(w, r, other.URL+"/doc", http.StatusFound)
		case "/headers":
			w.Header().Set("X-Pad", strings.Repeat("a", updatecheck.MaxHeaderBytes))
			_, _ = w.Write([]byte("padded"))
		default:
			_, _ = w.Write([]byte("doc"))
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	if b, err := updatecheck.DefaultGet(ctx, srv.URL+"/same", 100); err != nil || string(b) != "doc" {
		t.Fatalf("same-origin redirect: got %q, %v", b, err)
	}
	if _, err := updatecheck.DefaultGet(ctx, srv.URL+"/cross", 100); !errors.Is(err, updatecheck.ErrCrossOriginRedirect) {
		t.Fatalf("cross-origin redirect: got %v, want %v", err, updatecheck.ErrCrossOriginRedirect)
	}
	if _, err := updatecheck.DefaultGet(ctx, srv.URL+"/headers", 100); err == nil {
		t.Fatal("accepted oversized response headers")
	}
}

func TestDisabled(t *testing.T) {
	t.Parallel()

	reg := newRegistry()
	for _, tc := range []struct {
		name string
		o    updatecheck.Options
	}{
		{"no url", updatecheck.Options{}},
		// The runner's registry does not enable the check by itself.
		{"only the runner registry", func() updatecheck.Options { o := runnerOptions(reg); o.URL = ""; return o }()},
	} {
		s, err := updatecheck.New(log.Noop, tc.o)
		if s != nil || err != nil {
			t.Fatalf("%s: got %v, %v, want disabled", tc.name, s, err)
		}
	}
}

func TestInvalidOptions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		o    updatecheck.Options
	}{
		{"scheme", updatecheck.Options{URL: "ftp://registry.example"}},
		{"no host", updatecheck.Options{URL: "https:///path"}},
		{"negative interval", updatecheck.Options{URL: registryURL, Interval: -time.Second}},
	} {
		if _, err := updatecheck.New(log.Noop, tc.o); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}

// Every check, not only the first, is jittered.
func TestPollInterval(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		reg := newRegistry()
		reg.offer(t, release{version: 900, tags: []string{"2.8.0"}})
		o := runnerOptions(reg)
		o.Interval = 10 * time.Minute

		s, err := updatecheck.New(log.Noop, o)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		times := reg.requestTimes("/release.json")
		if len(times) < 16 {
			t.Fatalf("got %d checks in 3h", len(times))
		}
		if first := times[0].Sub(times[0].Truncate(24 * time.Hour)); first < 30*time.Second || first > time.Minute {
			t.Fatalf("first check after %s", first)
		}
		gaps := map[time.Duration]bool{}
		for i := 1; i < len(times); i++ {
			gap := times[i].Sub(times[i-1])
			if gap < 9*time.Minute || gap > 11*time.Minute {
				t.Fatalf("check %d after %s", i, gap)
			}
			gaps[gap] = true
		}
		if len(gaps) < 2 {
			t.Fatal("checks are not jittered")
		}
	})
}

// A newer release on another channel is logged once, the release notes
// shortened.
func TestOtherChannelLoggedOnce(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		reg := newRegistry()
		notes := strings.Repeat("n", 5000)
		reg.offer(t, release{version: 2000, channels: []string{"beta"}, tags: []string{"2.9.0"}, notes: notes})
		logger, out := logs()

		s, err := updatecheck.New(logger, runnerOptions(reg))
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		time.Sleep(time.Hour)
		synctest.Wait()

		logged := out.String()
		if n := strings.Count(logged, "a newer bee release exists on another channel"); n != 1 {
			t.Fatalf("logged %d times:\n%s", n, logged)
		}
		if strings.Contains(logged, notes) {
			t.Fatal("release notes not shortened")
		}
		if got := s.MetricValues(); got.Available != 0 || got.LatestVersion != 2000 {
			t.Fatalf("metrics: %+v", got)
		}
	})
}
