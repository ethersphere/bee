// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

const (
	feedMetadataEntryOwner = "swarm-feed-owner"
	feedMetadataEntryTopic = "swarm-feed-topic"
	feedMetadataEntryType  = "swarm-feed-type"
)

var (
	errInvalidFeedUpdate = errors.New("invalid feed update")
)

type feedReferenceResponse struct {
	Reference swarm.Address `json: "reference"`
}

func (s *server) feedGetHandler(w http.ResponseWriter, r *http.Request) {
	owner, err := hex.DecodeString(mux.Vars(r)["owner"])
	if err != nil {
		s.Logger.Debugf("feed get: decode owner: %v", err)
		s.Logger.Error("feed get: bad owner")
		jsonhttp.BadRequest(w, "bad owner")
		return
	}

	topic, err := hex.DecodeString(mux.Vars(r)["topic"])
	if err != nil {
		s.Logger.Debugf("feed get: decode topic: %v", err)
		s.Logger.Error("feed get: bad topic")
		jsonhttp.BadRequest(w, "bad topic")
		return
	}
	feedType := new(feeds.Type)
	typeStr := r.URL.Query().Get("type")
	if typeStr != "" {
		if err := feedType.FromString(typeStr); err != nil {
			s.Logger.Debugf("feed get: unknown type: %v", err)
			s.Logger.Error("feed get: unknown type")
			jsonhttp.BadRequest(w, "unknown type")
			return
		}
		if *feedType == feeds.Epoch {
			// throw some error for now
			s.Logger.Debugf("feed put: unsupported")
			s.Logger.Error("feed put: unsupported")
			jsonhttp.BadRequest(w, "unsupported")
			return
		}
	}

	var at int64
	atStr := r.URL.Query().Get("at")
	if atStr != "" {
		at, err = strconv.ParseInt(atStr, 10, 64)
		if err != nil {
			panic(err)
		}
	} else {
		at = time.Now().Unix()
	}

	f, err := feeds.New(string(topic), common.BytesToAddress(owner))
	if err != nil {
		panic(err)
	}
	lookup, err := s.feedFactory.NewLookup(*feedType, f)
	if err != nil {
		panic(err)
	}

	ch, _, _, err := lookup.At(r.Context(), int64(at), 0)
	if err != nil {
		panic(err)
	}

	ref, ts, err := parseFeedUpdate(ch)
	if err != nil {
		panic(err)
	}

	tsB := make([]byte, 8)
	binary.BigEndian.PutUint64(tsB, uint64(ts))
	id := hex.EncodeToString(tsB)
	w.Header().Set(SwarmFeedIndexHeader, id)
	w.Header().Set("Access-Control-Expose-Headers", SwarmFeedIndexHeader)

	jsonhttp.OK(w, feedReferenceResponse{Reference: ref})
}

func (s *server) feedPutHandler(w http.ResponseWriter, r *http.Request) {
	owner, err := hex.DecodeString(mux.Vars(r)["owner"])
	if err != nil {
		s.Logger.Debugf("feed put: decode owner: %v", err)
		s.Logger.Error("feed put: bad owner")
		jsonhttp.BadRequest(w, "bad owner")
		return
	}

	topic, err := hex.DecodeString(mux.Vars(r)["topic"])
	if err != nil {
		s.Logger.Debugf("feed put: decode topic: %v", err)
		s.Logger.Error("feed put: bad topic")
		jsonhttp.BadRequest(w, "bad topic")
		return
	}

	feedType := new(feeds.Type)
	typeStr := r.URL.Query().Get("type")
	if typeStr != "" {
		if err := feedType.FromString(typeStr); err != nil {
			s.Logger.Debugf("feed put: unknown type: %v", err)
			s.Logger.Error("feed put: unknown type")
			jsonhttp.BadRequest(w, "unknown type")
			return
		}
		if *feedType == feeds.Epoch {
			// throw some error for now
			s.Logger.Debugf("feed put: unsupported")
			s.Logger.Error("feed put: unsupported")
			jsonhttp.BadRequest(w, "unsupported")
			return
		}
	}
	var ref swarm.Address
	refStr := mux.Vars(r)["reference"]
	ref, err = swarm.ParseHexAddress(refStr)
	if err != nil {
		s.Logger.Debugf("feed put: parse reference: %v", err)
		s.Logger.Error("feed put: bad reference")
		jsonhttp.BadRequest(w, "bad reference")
		return
	}

	var at int64
	atStr := r.URL.Query().Get("at")
	if atStr != "" {
		at, err = strconv.ParseInt(atStr, 10, 64)
		if err != nil {
			panic(err)
		}
	} else {
		at = time.Now().Unix()
	}

	sig, err := hex.DecodeString(r.URL.Query().Get("sig"))
	if err != nil {
		s.Logger.Debugf("feed put: decode sig: %v", err)
		s.Logger.Error("feed put: bad signature")
		jsonhttp.BadRequest(w, "bad signature")
		return
	}

	f, err := feeds.New(string(topic), common.BytesToAddress(owner))
	if err != nil {
		panic(err)
	}
	// we need to know where to put the update, do a lookup then create the index
	lookup, err := s.feedFactory.NewLookup(*feedType, f)
	if err != nil {
		panic(err)
	}

	ch, _, next, err := lookup.At(r.Context(), int64(at), 0)
	if err != nil {
		panic(err)
	}

	//	we have the ref, we need to make the content addressed chunk out of it, with the timestamp
	//	then we need to create the soc with that owner and with that cac, then PUT that chunk
	// and do a recovery on the signature, to see that it matches the owner
	mode := requestModePut(r)
	ch, err = feeds.NewUpdate(f, next, at, ref.Bytes(), sig)
	if err != nil {
		panic(err)
	}
	_, err = s.Storer.Put(r.Context(), mode, ch)
	if err != nil {
		panic(err)
	}
	b, err := next.MarshalBinary()
	if err != nil {
		panic(err)
	}

	w.Header().Set(SwarmFeedIndexHeader, hex.EncodeToString(b))
	w.Header().Set("Access-Control-Expose-Headers", SwarmFeedIndexHeader)

	jsonhttp.OK(w, feedReferenceResponse{Reference: ref})
}

func (s *server) feedPostHandler(w http.ResponseWriter, r *http.Request) {
	owner, err := hex.DecodeString(mux.Vars(r)["owner"])
	if err != nil {
		s.Logger.Debugf("feed put: decode owner: %v", err)
		s.Logger.Error("feed put: bad owner")
		jsonhttp.BadRequest(w, "bad owner")
		return
	}

	topic, err := hex.DecodeString(mux.Vars(r)["topic"])
	if err != nil {
		s.Logger.Debugf("feed put: decode topic: %v", err)
		s.Logger.Error("feed put: bad topic")
		jsonhttp.BadRequest(w, "bad topic")
		return
	}
	feedType := new(feeds.Type)
	typeStr := r.URL.Query().Get("type")
	if typeStr != "" {
		if err := feedType.FromString(typeStr); err != nil {
			s.Logger.Debugf("feed put: unknown type: %v", err)
			s.Logger.Error("feed put: unknown type")
			jsonhttp.BadRequest(w, "unknown type")
			return
		}
		if *feedType == feeds.Epoch {
			// throw some error for now
			s.Logger.Debugf("feed put: unsupported")
			s.Logger.Error("feed put: unsupported")
			jsonhttp.BadRequest(w, "unsupported")
			return
		}
	}
	l := loadsave.New(s.Storer, requestModePut(r), false)
	feedManifest, err := manifest.NewDefaultManifest(l, false)
	meta := map[string]string{
		feedMetadataEntryOwner: hex.EncodeToString(owner),
		feedMetadataEntryTopic: hex.EncodeToString(topic),
		feedMetadataEntryType:  feedType.String(),
	}

	emptyAddr := make([]byte, 32, 32)
	err = feedManifest.Add(r.Context(), "/", manifest.NewEntry(swarm.NewAddress(emptyAddr), meta))
	if err != nil {
		s.Logger.Debugf("feed post: add manifest entry: %v", err)
		s.Logger.Error("feed post: add manifest entry")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	ref, err := feedManifest.Store(r.Context())
	if err != nil {
		s.Logger.Debugf("feed post: store manifest: %v", err)
		s.Logger.Error("feed post: store manifest")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	jsonhttp.Created(w, feedReferenceResponse{Reference: ref})
}

func generateUpdate(ref swarm.Address, ts uint64) swarm.Chunk {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	tb := make([]byte, swarm.SpanSize)
	binary.BigEndian.PutUint64(tb, ts)
	tb = append(tb, ref.Bytes()...)
	span := uint64(len(tb))

	b := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(b, span)
	err := hasher.SetSpanBytes(b)
	if err != nil {
		panic(err)
	}
	_, err = hasher.Write(b)
	if err != nil {
		panic(err)
	}
	s := swarm.NewAddress(hasher.Sum(nil))
	return swarm.NewChunk(s, append(b, tb...))

}

func parseFeedUpdate(ch swarm.Chunk) (swarm.Address, int64, error) {
	update := ch.Data()
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
