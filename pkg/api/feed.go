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
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/soc"
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
		At int64 `map:"at"`
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
		logger.Debug("new lookup failed", "owner", paths.Owner, "error", err)
		logger.Error(nil, "new lookup failed")
		jsonhttp.InternalServerError(w, "new lookup failed")
		return
	}

	ch, cur, next, err := lookup.At(r.Context(), queries.At, 0)
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

	ref, _, err := parseFeedUpdate(ch)
	if err != nil {
		logger.Debug("mapStructure feed update failed", "error", err)
		logger.Error(nil, "mapStructure feed update failed")
		jsonhttp.InternalServerError(w, "mapStructure feed update failed")
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

	putter, wait, err := s.newStamperPutter(r)
	if err != nil {
		logger.Debug("putter failed", "error", err)
		logger.Error(nil, "putter failed")
		switch {
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.BadRequest(w, "batch not found")
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable yet")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid postage batch id")
		default:
			jsonhttp.BadRequest(w, nil)
		}
		return
	}

	l := loadsave.New(putter, requestPipelineFactory(r.Context(), putter, r))
	feedManifest, err := manifest.NewDefaultManifest(l, false)
	if err != nil {
		logger.Debug("create manifest failed", "error", err)
		logger.Error(nil, "create manifest failed")
		jsonhttp.InternalServerError(w, "create manifest failed")
		return
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
		jsonhttp.InternalServerError(w, "add manifest entry failed")
		return
	}
	ref, err := feedManifest.Store(r.Context())
	if err != nil {
		logger.Debug("store manifest failed", "error", err)
		logger.Error(nil, "store manifest failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, "store manifest failed")
		}
		return
	}

	if requestPin(r) {
		if err := s.pinning.CreatePin(r.Context(), ref, false); err != nil {
			logger.Debug("pin creation failed: %v", "address", ref, "error", err)
			logger.Error(nil, "pin creation failed")
			jsonhttp.InternalServerError(w, "creation of pin failed")
			return
		}
	}

	if err = wait(); err != nil {
		logger.Debug("sync chunks failed", "error", err)
		logger.Error(nil, "sync chunks failed")
		jsonhttp.InternalServerError(w, "feed upload: sync failed")
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
