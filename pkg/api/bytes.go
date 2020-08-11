// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	tag, err := s.getOrCreateTag(r.Header.Get(TagHeaderUid))
	if err != nil {
		s.Logger.Debugf("bytes upload: get or create tag: %v", err)
		s.Logger.Error("bytes upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)

	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	sp := splitter.NewSimpleSplitter(s.Storer)
	address, err := file.SplitWriteAll(ctx, sp, r.Body, r.ContentLength, toEncrypt)
	if err != nil {
		s.Logger.Debugf("bytes upload: split write all: %v", err)
		s.Logger.Error("bytes upload: split write all")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	w.Header().Set(TagHeaderUid, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderUid)
	jsonhttp.OK(w, bytesPostResponse{
		Reference: address,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *server) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bytes: parse address %s: %v", addressHex, err)
		s.Logger.Error("bytes: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	additionalHeaders := http.Header{
		"Content-Type": {"application/octet-stream"},
	}

	s.downloadHandler(w, r, address, additionalHeaders)
}

func (s *server) getOrCreateTag(tagUid string) (*tags.Tag, error) {
	// if tag header is not there create a new one
	if tagUid == "" {
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		var err error
		tag, err := s.Tags.Create(tagName, 0, false)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag: %w", err)
		}
		return tag, nil
	}
	// if the tag uid header is present, then use the tag sent
	uid, err := strconv.Atoi(tagUid)
	if err != nil {
		return nil, fmt.Errorf("cannot parse taguid: %w", err)
	}

	return s.Tags.Get(uint32(uid))
}
