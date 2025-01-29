// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/v2/pkg/manifest/simple"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	feedMetadataEntryOwner = "swarm-feed-owner"
	feedMetadataEntryTopic = "swarm-feed-topic"
	feedMetadataEntryType  = "swarm-feed-type"
)

type feedReferenceResponse struct {
	Reference swarm.Address `json:"reference"`
}

func (s *Service) feedGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_feed").Build()

	paths := struct {
		Owner common.Address `map:"owner" validate:"required"`
		Topic []byte         `map:"topic" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	queries := struct {
		At    int64  `map:"at"`
		After uint64 `map:"after"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}
	if queries.At == 0 {
		queries.At = time.Now().Unix()
	}

	headers := struct {
		OnlyRootChunk bool `map:"Swarm-Only-Root-Chunk"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	f := feeds.New(paths.Topic, paths.Owner)
	lookup, err := s.feedFactory.NewLookup(feeds.Sequence, f)
	if err != nil {
		logger.Debug("new lookup failed", "owner", paths.Owner, "error", err)
		logger.Error(nil, "new lookup failed")
		switch {
		case errors.Is(err, feeds.ErrFeedTypeNotFound):
			jsonhttp.NotFound(w, "feed type not found")
		default:
			jsonhttp.InternalServerError(w, "new lookup failed")
		}
		return
	}

	ch, cur, next, err := lookup.At(r.Context(), queries.At, queries.After)
	if err != nil {
		logger.Debug("lookup at failed", "at", queries.At, "error", err)
		logger.Error(nil, "lookup at failed")
		jsonhttp.NotFound(w, "lookup at failed")
		return
	}

	// KLUDGE: if a feed was never updated, the chunk will be nil
	if ch == nil {
		logger.Debug("no update found")
		logger.Error(nil, "no update found")
		jsonhttp.NotFound(w, "no update found")
		return
	}

	wc, err := feeds.GetWrappedChunk(r.Context(), s.storer.Download(false), ch)
	if err != nil {
		logger.Error(nil, "wrapped chunk cannot be retrieved")
		jsonhttp.NotFound(w, "wrapped chunk cannot be retrieved")
		return
	}

	curBytes, err := cur.MarshalBinary()
	if err != nil {
		logger.Debug("marshal current index failed", "error", err)
		logger.Error(nil, "marshal current index failed")
		jsonhttp.InternalServerError(w, "marshal current index failed")
		return
	}

	nextBytes, err := next.MarshalBinary()
	if err != nil {
		logger.Debug("marshal next index failed", "error", err)
		logger.Error(nil, "marshal next index failed")
		jsonhttp.InternalServerError(w, "marshal next index failed")
		return
	}

	socCh, err := soc.FromChunk(ch)
	if err != nil {
		logger.Error(nil, "wrapped chunk cannot be retrieved")
		jsonhttp.NotFound(w, "wrapped chunk cannot be retrieved")
		return
	}
	sig := socCh.Signature()

	additionalHeaders := http.Header{
		ContentTypeHeader:          {"application/octet-stream"},
		SwarmFeedIndexHeader:       {hex.EncodeToString(curBytes)},
		SwarmFeedIndexNextHeader:   {hex.EncodeToString(nextBytes)},
		SwarmSocSignatureHeader:    {hex.EncodeToString(sig)},
		AccessControlExposeHeaders: {SwarmFeedIndexHeader, SwarmFeedIndexNextHeader, SwarmSocSignatureHeader},
	}

	if headers.OnlyRootChunk {
		w.Header().Set(ContentLengthHeader, strconv.Itoa(len(wc.Data())))
		// include additional headers
		for name, values := range additionalHeaders {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		_, _ = io.Copy(w, bytes.NewReader(wc.Data()))
		return
	}

	s.downloadHandler(logger, w, r, wc.Address(), additionalHeaders, true, false, wc)
}

func (s *Service) feedPostHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_feed").Build()

	paths := struct {
		Owner common.Address `map:"owner" validate:"required"`
		Topic []byte         `map:"topic" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		BatchID        []byte        `map:"Swarm-Postage-Batch-Id" validate:"required"`
		Pin            bool          `map:"Swarm-Pin"`
		Deferred       *bool         `map:"Swarm-Deferred-Upload"`
		Act            bool          `map:"Swarm-Act"`
		HistoryAddress swarm.Address `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag      storer.SessionInfo
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)
	if deferred || headers.Pin {
		tag, err = s.storer.NewSession()
		if err != nil {
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			return
		}
	}

	putter, err := s.newStamperPutter(r.Context(), putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag.TagID,
		Pin:      headers.Pin,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("get putter failed", "error", err)
		logger.Error(nil, "get putter failed")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	ow := &cleanupOnErrWriter{
		ResponseWriter: w,
		onErr:          putter.Cleanup,
		logger:         logger,
	}

	l := loadsave.New(s.storer.ChunkStore(), s.storer.Cache(), requestPipelineFactory(r.Context(), putter, false, 0))
	feedManifest, err := manifest.NewDefaultManifest(l, false)
	if err != nil {
		logger.Debug("create manifest failed", "error", err)
		logger.Error(nil, "create manifest failed")
		switch {
		case errors.Is(err, manifest.ErrInvalidManifestType):
			jsonhttp.BadRequest(ow, "invalid manifest type")
		default:
			jsonhttp.InternalServerError(ow, "create manifest failed")
		}
	}

	meta := map[string]string{
		feedMetadataEntryOwner: hex.EncodeToString(paths.Owner.Bytes()),
		feedMetadataEntryTopic: hex.EncodeToString(paths.Topic),
		feedMetadataEntryType:  feeds.Sequence.String(), // only sequence allowed for now
	}

	emptyAddr := make([]byte, 32)

	// a feed manifest stores the metadata at the root "/" path
	err = feedManifest.Add(r.Context(), "/", manifest.NewEntry(swarm.NewAddress(emptyAddr), meta))
	if err != nil {
		logger.Debug("add manifest entry failed", "error", err)
		logger.Error(nil, "add manifest entry failed")
		switch {
		case errors.Is(err, simple.ErrEmptyPath):
			jsonhttp.NotFound(ow, "invalid or empty path")
		case errors.Is(err, mantaray.ErrEmptyPath):
			jsonhttp.NotFound(ow, "invalid path or mantaray path is empty")
		default:
			jsonhttp.InternalServerError(ow, "add manifest entry failed")
		}
		return
	}
	ref, err := feedManifest.Store(r.Context())
	if err != nil {
		logger.Debug("store manifest failed", "error", err)
		logger.Error(nil, "store manifest failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(ow, "batch is overissued")
		default:
			jsonhttp.InternalServerError(ow, "store manifest failed")
		}
		return
	}

	encryptedReference := ref
	if headers.Act {
		encryptedReference, err = s.actEncryptionHandler(r.Context(), w, putter, ref, headers.HistoryAddress)
		if err != nil {
			logger.Debug("access control upload failed", "error", err)
			logger.Error(nil, "access control upload failed")
			switch {
			case errors.Is(err, accesscontrol.ErrNotFound):
				jsonhttp.NotFound(w, "act or history entry not found")
			case errors.Is(err, accesscontrol.ErrInvalidPublicKey) || errors.Is(err, accesscontrol.ErrSecretKeyInfinity):
				jsonhttp.BadRequest(w, "invalid public key")
			case errors.Is(err, accesscontrol.ErrUnexpectedType):
				jsonhttp.BadRequest(w, "failed to create history")
			default:
				jsonhttp.InternalServerError(w, errActUpload)
			}
			return
		}
	}

	err = putter.Done(ref)
	if err != nil {
		logger.Debug("done split failed", "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(ow, "done split failed")
		return
	}

	jsonhttp.Created(w, feedReferenceResponse{Reference: encryptedReference})
}
