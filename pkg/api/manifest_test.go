// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fakeRef derives a deterministic non-zero 32-byte reference from a path. The
// listing endpoint returns entry references straight from the trie without
// dereferencing them, so a synthetic address is sufficient for these tests.
func fakeRef(path string) swarm.Address {
	b := make([]byte, swarm.HashSize)
	copy(b, path)
	b[swarm.HashSize-1] = 0x01 // guarantee non-zero even for the empty path
	return swarm.NewAddress(b)
}

// buildManifest writes a mantaray manifest containing the given paths to the
// storer and returns its root reference.
func buildManifest(t *testing.T, storer api.Storer, paths []string) swarm.Address {
	t.Helper()
	ctx := context.Background()

	m, err := manifest.NewDefaultManifest(
		loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false, 0), redundancy.DefaultDownloadLevel),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	// bee's uploader stores manifest-level metadata at RootPath with an empty
	// (32-zero-byte) reference; the listing must not surface it as a file entry.
	err = m.Add(ctx, manifest.RootPath, manifest.NewEntry(
		swarm.NewAddress(make([]byte, swarm.HashSize)),
		map[string]string{"website-index-document": "index.html"},
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		err = m.Add(ctx, p, manifest.NewEntry(fakeRef(p), map[string]string{
			manifest.EntryMetadataContentTypeKey: "application/octet-stream",
			manifest.EntryMetadataFilenameKey:    p,
		}))
		if err != nil {
			t.Fatal(err)
		}
	}
	ref, err := m.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func manifestPaths(resp api.ManifestListResponse) []string {
	out := make([]string, len(resp.Entries))
	for i, e := range resp.Entries {
		out[i] = e.Path
	}
	return out
}

func TestManifestList(t *testing.T) {
	t.Parallel()

	storer := mockstorer.New()
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: storer,
		Logger: log.Noop,
	})

	files := []string{
		"index.html",
		"data/a.parquet",
		"data/b.parquet",
		"data/2025/x.parquet",
		"docs/readme.md",
	}
	ref := buildManifest(t, storer, files)

	list := func(t *testing.T, url string) api.ManifestListResponse {
		t.Helper()
		var resp api.ManifestListResponse
		jsonhttptest.Request(t, client, http.MethodGet, url, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		return resp
	}

	t.Run("recursive from root", func(t *testing.T) {
		resp := list(t, fmt.Sprintf("/manifest/%s/", ref))
		want := []string{
			"data/2025/x.parquet",
			"data/a.parquet",
			"data/b.parquet",
			"docs/readme.md",
			"index.html",
		}
		got := manifestPaths(resp)
		if !equalStrings(got, want) {
			t.Fatalf("recursive listing mismatch:\n got %v\nwant %v", got, want)
		}
		if resp.Truncated {
			t.Fatalf("did not expect truncation")
		}
		if len(resp.CommonPrefixes) != 0 {
			t.Fatalf("did not expect common prefixes, got %v", resp.CommonPrefixes)
		}
		// metadata should round-trip
		if ct := resp.Entries[0].Metadata[manifest.EntryMetadataContentTypeKey]; ct != "application/octet-stream" {
			t.Fatalf("unexpected content-type metadata: %q", ct)
		}
	})

	t.Run("delimiter shallow listing", func(t *testing.T) {
		resp := list(t, fmt.Sprintf("/manifest/%s/?delimiter=/", ref))
		if got := manifestPaths(resp); !equalStrings(got, []string{"index.html"}) {
			t.Fatalf("shallow entries mismatch: got %v", got)
		}
		if got := resp.CommonPrefixes; !equalStrings(got, []string{"data/", "docs/"}) {
			t.Fatalf("common prefixes mismatch: got %v", got)
		}
	})

	t.Run("prefix", func(t *testing.T) {
		resp := list(t, fmt.Sprintf("/manifest/%s/data/", ref))
		want := []string{"data/2025/x.parquet", "data/a.parquet", "data/b.parquet"}
		if got := manifestPaths(resp); !equalStrings(got, want) {
			t.Fatalf("prefix listing mismatch:\n got %v\nwant %v", got, want)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		var all []string
		url := fmt.Sprintf("/manifest/%s/?limit=2", ref)
		for {
			resp := list(t, url)
			all = append(all, manifestPaths(resp)...)
			if !resp.Truncated {
				break
			}
			if resp.NextMarker == "" {
				t.Fatal("truncated page without a nextMarker")
			}
			url = fmt.Sprintf("/manifest/%s/?limit=2&after=%s", ref, resp.NextMarker)
		}
		want := []string{
			"data/2025/x.parquet",
			"data/a.parquet",
			"data/b.parquet",
			"docs/readme.md",
			"index.html",
		}
		if !equalStrings(all, want) {
			t.Fatalf("paginated listing mismatch:\n got %v\nwant %v", all, want)
		}
	})

	t.Run("missing prefix is 404", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet,
			fmt.Sprintf("/manifest/%s/nope/", ref), http.StatusNotFound)
	})

	t.Run("limit above cap is 400", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet,
			fmt.Sprintf("/manifest/%s/?limit=1000001", ref), http.StatusBadRequest)
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
