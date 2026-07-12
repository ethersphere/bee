// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/v2/contracts"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// envelopeDownload returns the configured getter for envelope upload/download paths.
func (s *Service) envelopeDownload(cache bool) storage.Getter {
	if s.envelopeStorage != nil {
		return s.envelopeStorage.Download(cache)
	}
	return s.storer.Download(cache)
}

// envelopeCache returns the configured cache putter.
func (s *Service) envelopeCache() storage.Putter {
	if s.envelopeStorage != nil {
		return s.envelopeStorage.Cache()
	}
	return s.storer.Cache()
}

// envelopeChunkStore returns the configured read-only chunk store for envelope paths.
func (s *Service) envelopeChunkStore() storage.ReadOnlyChunkStore {
	if s.envelopeStorage != nil {
		return s.envelopeStorage.ChunkStore()
	}
	return s.storer.ChunkStore()
}

// envelopeDirectUpload opens a DirectUpload session (cache + pushsync).
func (s *Service) envelopeDirectUpload() (storer.PutterSession, error) {
	if s.envelopeStorage == nil {
		return s.storer.DirectUpload(), nil
	}
	return s.envelopeStorage.DirectUpload()
}

// envelopeUpload opens a deferred/pin upload session.
func (s *Service) envelopeUpload(ctx context.Context, pin bool, tagID uint64) (storer.PutterSession, error) {
	if s.envelopeStorage != nil {
		return s.envelopeStorage.Upload(ctx, pin, tagID)
	}
	return s.storer.Upload(ctx, pin, tagID)
}

// envelopeNewCollection opens a pin collection session.
func (s *Service) envelopeNewCollection(ctx context.Context) (storer.PutterSession, error) {
	if s.envelopeStorage != nil {
		return s.envelopeStorage.NewCollection(ctx)
	}
	return s.storer.NewCollection(ctx)
}

func (s *Service) envelopeStamperPutter(ctx context.Context, opts putterOptions) (storer.PutterSession, error) {
	if opts.Deferred || opts.Pin {
		s.logGrpcFileUpload("upload-store", opts)
		return s.envelopeUpload(ctx, opts.Pin, opts.TagID)
	}
	s.logGrpcFileUpload("direct-upload", opts)
	session, err := s.envelopeDirectUpload()
	if err != nil {
		return nil, fmt.Errorf("envelope direct upload: %w", err)
	}
	return session, nil
}

func (s *Service) logGrpcFileUpload(session string, opts putterOptions) {
	if s.envelopeStorage == nil {
		return
	}
	s.logger.Info(contracts.LogMarkerUpload,
		"transport", "grpc",
		"session", session,
		"deferred", opts.Deferred,
		"pin", opts.Pin,
		"tag_id", opts.TagID,
	)
}

func (s *Service) logGrpcFileDownload(endpoint string, reference swarm.Address, cache bool) {
	if s.envelopeStorage == nil {
		return
	}
	s.logger.Info(contracts.LogMarkerDownload,
		"transport", "grpc",
		"endpoint", endpoint,
		"reference", reference,
		"cache_on_miss", cache,
	)
}
