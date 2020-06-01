// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

func (s *server) getANewTag(w http.ResponseWriter, r *http.Request) {
	tagName := mux.Vars(r)["name"]
	if tagName == "" {
		s.Logger.Debugf("bzz-tag: invalid tag name : %s", tagName)
		s.Logger.Error("bzz-tag: invalid tag name")
		jsonhttp.BadRequest(w, "invalid tag name")
		return
	}

	tag, err := s.Tags.Create(tagName, 1, false)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: tag creation error: %v", err)
		s.Logger.Error("bzz-chunk: tag creation error")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}

	jsonhttp.OK(w, tag)
}

func (s *server) getTagInfoUsingAddress(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-tag: parse chunk address %s: %v", addr, err)
		s.Logger.Error("bzz-tag: error uploading chunk")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	tag, err := s.Tags.GetByAddress(address)
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag not present %s : %v, ", address.String(), err)
		s.Logger.Error("bzz-tag: tag not present")
		jsonhttp.InternalServerError(w, "tag not present")
		return
	}

	w.Header().Set("Content-Type", jsonhttp.DefaultContentTypeHeader)
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	err = json.NewEncoder(w).Encode(&tag)
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag encode error %s: %v", address.String(), err)
		s.Logger.Error("bzz-tag: tag encode error")
		jsonhttp.InternalServerError(w, "tag encode error")
		return
	}

	jsonhttp.OK(w, tag)
}

func (s *server) getTagInfoUsingUUid(w http.ResponseWriter, r *http.Request) {
	uidStr := mux.Vars(r)["uuid"]

	uuid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		s.Logger.Debugf("bzz-tag: parse uuid  %s: %v", uidStr, err)
		s.Logger.Error("bzz-tag: error uploading chunk")
		jsonhttp.BadRequest(w, "invalid chunk address")
		return
	}

	tag, err := s.Tags.Get(uint32(uuid))
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag not present : %v, uuid %s", err, uidStr)
		s.Logger.Error("bzz-tag: tag not present")
		jsonhttp.InternalServerError(w, "tag not present")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	err = json.NewEncoder(w).Encode(&tag)
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag encode error: %v, uuid %s", err, uidStr)
		s.Logger.Error("bzz-tag: tag encode error")
		jsonhttp.InternalServerError(w, "tag encode error")
		return
	}

	jsonhttp.OK(w, tag)
}
