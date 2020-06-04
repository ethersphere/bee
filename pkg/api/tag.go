// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"

	"github.com/gorilla/mux"
)

type tagResponse struct {
	Total     int64         `json:"total"`
	Split     int64         `json:"split"`
	Seen      int64         `json:"seen"`
	Stored    int64         `json:"stored"`
	Sent      int64         `json:"sent"`
	Synced    int64         `json:"synced"`
	Uid       uint32        `json:"uid"`
	Anonymous bool          `json:"anonymous"`
	Name      string        `json:"name"`
	Address   swarm.Address `json:"address"`
	StartedAt time.Time     `json:"startedAt"`
}

func newTagResponse(tag *tags.Tag) tagResponse {
	return tagResponse{
		Total:     tag.Total,
		Split:     tag.Split,
		Seen:      tag.Seen,
		Stored:    tag.Stored,
		Sent:      tag.Sent,
		Synced:    tag.Synced,
		Uid:       tag.Uid,
		Anonymous: tag.Anonymous,
		Name:      tag.Name,
		Address:   tag.Address,
		StartedAt: tag.StartedAt,
	}
}
func (s *server) CreateTag(w http.ResponseWriter, r *http.Request) {
	tagName := mux.Vars(r)["name"]

	tag, err := s.Tags.Create(tagName, 0, false)
	if err != nil {
		s.Logger.Debugf("bzz-chunk: tag create error: %v", err)
		s.Logger.Error("bzz-chunk: tag create error")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))

}

func (s *server) getTagInfoUsingAddress(w http.ResponseWriter, r *http.Request) {
	addr := mux.Vars(r)["addr"]
	address, err := swarm.ParseHexAddress(addr)
	if err != nil {
		s.Logger.Debugf("bzz-tag: parse chunk address %s: %v", addr, err)
		s.Logger.Error("bzz-tag: parse chunk address")
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

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}

func (s *server) getTagInfoUsingUUid(w http.ResponseWriter, r *http.Request) {
	uidStr := mux.Vars(r)["uuid"]

	uuid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		s.Logger.Debugf("bzz-tag: parse uid  %s: %v", uidStr, err)
		s.Logger.Error("bzz-tag: parse uid")
		jsonhttp.BadRequest(w, "invalid uid")
		return
	}

	tag, err := s.Tags.Get(uint32(uuid))
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag not present : %v, uuid %s", err, uidStr)
		s.Logger.Error("bzz-tag: tag not present")
		jsonhttp.InternalServerError(w, "tag not present")
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}
