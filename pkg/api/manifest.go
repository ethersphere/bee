// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/gorilla/mux"
)

const (
	manifestListDefaultLimit = 1000
	manifestListMaxLimit     = 10000
)

// emptyManifestEntry is the reference bee's uploader stores for manifest-level
// metadata held at RootPath (e.g. website-index-document): 32 zero bytes. Note
// this is NOT swarm.ZeroAddress (which has nil bytes), so Address.IsZero() does
// not match it — mirror the convention used by mantarayManifest.IterateAddresses.
var emptyManifestEntry = swarm.NewAddress([]byte{31: 0})

type ManifestListEntry struct {
	Path      string            `json:"path"`
	Reference swarm.Address     `json:"reference"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Size      *int64            `json:"size,omitempty"`
}

type ManifestListResponse struct {
	Entries        []ManifestListEntry `json:"entries"`
	CommonPrefixes []string            `json:"commonPrefixes,omitempty"`
	Truncated      bool                `json:"truncated"`
	NextMarker     string              `json:"nextMarker,omitempty"`
}

// manifestListHandler serves a server-side listing of a manifest's contents,
// walking the Mantaray trie on the node instead of forcing clients to fetch
// and traverse it chunk by chunk. Semantics mirror S3 ListObjectsV2 (prefix,
// delimiter, pagination) — see ethersphere/bee#5535.
func (s *Service) manifestListHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_manifest").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
		Prefix  string        `map:"path"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	queries := struct {
		Delimiter string `map:"delimiter"`
		Limit     int    `map:"limit"`
		After     string `map:"after"`
		Sizes     bool   `map:"sizes"`
	}{
		Limit: manifestListDefaultLimit,
	}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}
	// enforce the node-side hard cap; a non-positive limit falls back to it too.
	if queries.Limit <= 0 || queries.Limit > manifestListMaxLimit {
		queries.Limit = manifestListMaxLimit
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	ctx := r.Context()
	ls := loadsave.NewReadonly(s.storer.Download(true), s.storer.Cache(), redundancy.DefaultDownloadLevel)

	m, err := manifest.NewDefaultManifestReference(address, ls)
	if err != nil {
		logger.Debug("manifest list: not a manifest", "address", address, "error", err)
		logger.Error(nil, "manifest list: not a manifest")
		jsonhttp.NotFound(w, nil)
		return
	}

	walker, ok := m.(manifest.EntryWalker)
	if !ok {
		logger.Error(nil, "manifest list: manifest type does not support listing")
		jsonhttp.InternalServerError(w, "manifest listing not supported")
		return
	}

	prefix := paths.Prefix
	resp := ManifestListResponse{Entries: []ManifestListEntry{}}
	seenPrefix := make(map[string]struct{})
	// marker tracks the last path fully consumed on this page; on truncation it
	// becomes nextMarker so a resumed page starts strictly after it. It is only
	// advanced for paths that are actually accounted for (emitted or folded into
	// a common prefix), never for entries skipped by the `after` continuation.
	var marker string

	walkErr := walker.WalkEntry(ctx, prefix, func(p string, entry manifest.Entry) error {
		// skip the synthetic root-metadata entry (empty reference); its
		// manifest-level metadata (e.g. website-index-document) is not a file.
		if entry.Reference().IsZero() || entry.Reference().Equal(emptyManifestEntry) {
			return nil
		}
		// continuation token: only paths strictly after it are unseen.
		if queries.After != "" && p <= queries.After {
			return nil
		}

		if queries.Delimiter != "" {
			rest := strings.TrimPrefix(p, prefix)
			if idx := strings.Index(rest, queries.Delimiter); idx >= 0 {
				cp := prefix + rest[:idx+len(queries.Delimiter)]
				if _, dup := seenPrefix[cp]; dup {
					// already an open common prefix — folding p in is free.
					marker = p
					return nil
				}
				if len(resp.Entries)+len(resp.CommonPrefixes) >= queries.Limit {
					resp.Truncated = true
					resp.NextMarker = marker
					return manifest.ErrStopWalk
				}
				seenPrefix[cp] = struct{}{}
				resp.CommonPrefixes = append(resp.CommonPrefixes, cp)
				marker = p
				return nil
			}
		}

		if len(resp.Entries)+len(resp.CommonPrefixes) >= queries.Limit {
			resp.Truncated = true
			resp.NextMarker = marker
			return manifest.ErrStopWalk
		}

		le := ManifestListEntry{
			Path:      p,
			Reference: entry.Reference(),
			Metadata:  entry.Metadata(),
		}
		if queries.Sizes {
			if size, err := s.manifestEntrySize(ctx, entry.Reference()); err == nil {
				le.Size = &size
			} else {
				// size is best-effort: a legacy/unreachable entry still lists.
				logger.Debug("manifest list: size resolution failed", "path", p, "error", err)
			}
		}
		resp.Entries = append(resp.Entries, le)
		marker = p
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, manifest.ErrNotFound) {
			logger.Debug("manifest list: prefix not found", "address", address, "prefix", prefix)
			jsonhttp.NotFound(w, "prefix not found")
			return
		}
		// a partially-retrievable manifest is reported as an error rather than
		// silently returning a truncated view (see #5535 error semantics).
		logger.Debug("manifest list: walk failed", "address", address, "error", walkErr)
		logger.Error(nil, "manifest list: walk failed")
		jsonhttp.NotFound(w, "manifest incomplete or not retrievable")
		return
	}

	jsonhttp.OK(w, resp)
}

// manifestEntrySize resolves a file entry's byte length by reading the 8-byte
// span header from its root chunk. Opt-in (sizes=true): one extra chunk read
// per entry.
func (s *Service) manifestEntrySize(ctx context.Context, ref swarm.Address) (int64, error) {
	ch, err := s.storer.Download(true).Get(ctx, ref)
	if err != nil {
		return 0, err
	}
	data := ch.Data()
	if len(data) < swarm.SpanSize {
		return 0, errors.New("chunk shorter than span")
	}
	return int64(binary.LittleEndian.Uint64(data[:swarm.SpanSize])), nil
}
