// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/manifest/mantaray"
	"github.com/ethersphere/bee/pkg/manifest/simple"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	feedMetadataEntryOwner = "swarm-feed-owner"
	feedMetadataEntryTopic = "swarm-feed-topic"
	feedMetadataEntryType  = "swarm-feed-type"
)

var errInvalidFeedUpdate = errors.New("invalid feed update")

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

	f := feeds.New(paths.Topic, paths.Owner)
	lookup, err := s.feedFactory.NewLookup(feeds.Sequence, f)
	if err != nil {
		logger.Debug("unable to create feed lookup", "topic", paths.Topic, "owner", paths.Owner, "error", err)
		logger.Warning("unable to create feed lookup")
		switch {
		case errors.Is(err, feeds.ErrFeedTypeNotFound):
			jsonhttp.NotFound(w, "feed type not found")
		default:
			jsonhttp.InternalServerError(w, "create feed lookup")
		}
		return
	}

	ch, cur, next, err := lookup.At(r.Context(), queries.At, queries.After)
	if err != nil {
		logger.Debug("unable to lookup", "at", queries.At, "after", queries.After, "error", err)
		logger.Warning("unable to lookup")
		jsonhttp.NotFound(w, "lookup")
		return
	}

	// KLUDGE: if a feed was never updated, the chunk will be nil
	if ch == nil {
		logger.Debug("no update found")
		logger.Warning("no update found")
		jsonhttp.NotFound(w, "no update found")
		return
	}

	ref, _, err := parseFeedUpdate(ch)
	if err != nil {
		logger.Debug("unable to parse feed update for chunk", "chunk", ch.Address(), "error", err)
		logger.Warning("unable to parse feed update for chunk")
		jsonhttp.InternalServerError(w, "feed update parse")
		return
	}

	curBytes, err := cur.MarshalBinary()
	if err != nil {
		logger.Debug("unable to marshal current index", "index", cur, "error", err)
		logger.Warning("unable to marshal current index")
		jsonhttp.InternalServerError(w, "marshal current index")
		return
	}

	nextBytes, err := next.MarshalBinary()
	if err != nil {
		logger.Debug("unable to marshal next index", "index", cur, "error", err)
		logger.Warning("unable to marshal next index")
		jsonhttp.InternalServerError(w, "marshal next index")
		return
	}

	w.Header().Set(SwarmFeedIndexHeader, hex.EncodeToString(curBytes))
	w.Header().Set(SwarmFeedIndexNextHeader, hex.EncodeToString(nextBytes))
	w.Header().Set("Access-Control-Expose-Headers", fmt.Sprintf("%s, %s", SwarmFeedIndexHeader, SwarmFeedIndexNextHeader))

	jsonhttp.OK(w, feedReferenceResponse{Reference: ref})
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
		BatchID  []byte `map:"Swarm-Postage-Batch-Id" validate:"required"`
		Pin      bool   `map:"Swarm-Pin"`
		Deferred *bool  `map:"Swarm-Deferred-Upload"`
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
			logger.Debug("unable to get/create tag", "error", err)
			logger.Warning("unable to get/create tag")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "get/create tag")
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
		logger.Debug("unable to get putter", "batch_id", headers.BatchID, "tag_id", tag, "pin", headers.Pin, "deferred", deferred, "error", err)
		logger.Warning("unable to get putter")
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

	l := loadsave.New(s.storer.ChunkStore(), requestPipelineFactory(r.Context(), putter, false))
	feedManifest, err := manifest.NewDefaultManifest(l, false)
	if err != nil {
		logger.Debug("unable to create manifest", "error", err)
		logger.Warning("unable to create manifest")
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
		logger.Debug("unable to add manifest entry", "metadata", meta, "error", err)
		logger.Warning("unable to add manifest entry")
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
		logger.Debug("unable to store manifest", "metadata", meta, "error", err)
		logger.Warning("unable to store manifest")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(ow, "batch is over-issued")
		default:
			jsonhttp.InternalServerError(ow, "store manifest failed")
		}
		return
	}

	err = putter.Done(ref)
	if err != nil {
		logger.Debug("unable to close session", "error", err)
		logger.Warning("unable to close session")
		jsonhttp.InternalServerError(ow, "unable to close session")
		return
	}

	jsonhttp.Created(w, feedReferenceResponse{Reference: ref})
}

func parseFeedUpdate(ch swarm.Chunk) (swarm.Address, int64, error) {
	s, err := soc.FromChunk(ch)
	if err != nil {
		return swarm.ZeroAddress, 0, fmt.Errorf("soc unmarshal: %w", err)
	}

	update := s.WrappedChunk().Data()
	// split the timestamp and reference
	// possible values right now:
	// unencrypted ref: span+timestamp+ref => 8+8+32=48
	// encrypted ref: span+timestamp+ref+decryptKey => 8+8+64=80
	if len(update) != 48 && len(update) != 80 {
		return swarm.ZeroAddress, 0, errInvalidFeedUpdate
	}
	ts := binary.BigEndian.Uint64(update[8:16])
	ref := swarm.NewAddress(update[16:])
	return ref, int64(ts), nil
}
