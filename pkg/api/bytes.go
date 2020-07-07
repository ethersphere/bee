// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	"strings"

	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ta := s.createTag(w, r)

	toEncrypt := strings.ToLower(r.Header.Get(EncryptHeader)) == "true"
	sp := splitter.NewSimpleSplitter(s.Storer)
	address, err := file.SplitWriteAll(ctx, sp, r.Body, r.ContentLength, toEncrypt)
	if err != nil {
		s.Logger.Debugf("bytes upload: %v", err)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	ta.Address = address

	w.Header().Set(TagHeaderUid, fmt.Sprint(ta.Uid))
	w.Header().Set("Access-Control-Expose-Headers", TagHeaderUid)
	jsonhttp.OK(w, bytesPostResponse{
		Reference: address,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *server) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("bytes: parse address %s: %v", addressHex, err)
		s.Logger.Error("bytes: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	toDecrypt := len(address.Bytes()) == (swarm.HashSize + encryption.KeyLength)
	j := joiner.NewSimpleJoiner(s.Storer)
	dataSize, err := j.Size(ctx, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("bytes: not found %s: %v", address, err)
			s.Logger.Error("bytes: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("bytes: invalid root chunk %s: %v", address, err)
		s.Logger.Error("bytes: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	outBuffer := bytes.NewBuffer(nil)
	c, err := file.JoinReadAll(j, address, outBuffer, toDecrypt)
	if err != nil && c == 0 {
		s.Logger.Debugf("bytes download: data join %s: %v", address, err)
		s.Logger.Errorf("bytes download: data join %s", address)
		jsonhttp.NotFound(w, nil)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf("%q", address))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	if _, err = io.Copy(w, outBuffer); err != nil {
		s.Logger.Debugf("bytes download: data read %s: %v", address, err)
		s.Logger.Errorf("bytes download: data read %s", address)
	}
}

func (s *server) createTag(w http.ResponseWriter, r *http.Request) *tags.Tag {
	// if tag header is not there create a new one
	var tag *tags.Tag
	tagUidStr := r.Header.Get(TagHeaderUid)
	if tagUidStr == "" {
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		var err error
		tag, err = s.Tags.Create(tagName, 0, false)
		if err != nil {
			s.Logger.Debugf("bytes upload: tag creation error: %v", err)
			s.Logger.Error("bytes upload: tag creation")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return nil
		}
	} else {
		// if the tag uid header is present, then use the tag sent
		tagUid, err := strconv.ParseUint(tagUidStr, 10, 32)
		if err != nil {
			s.Logger.Debugf("bytes upload: parse taguid %s: %v", tagUidStr, err)
			s.Logger.Error("bytes upload: parse taguid")
			jsonhttp.BadRequest(w, "invalid taguid")
			return nil
		}

		tag, err = s.Tags.Get(uint32(tagUid))
		if err != nil {
			s.Logger.Debugf("bytes upload: get tag error: %v", err)
			s.Logger.Error("bytes upload: get tag")
			jsonhttp.InternalServerError(w, "cannot create tag")
			return nil
		}
	}
	return tag
}
