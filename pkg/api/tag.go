// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"

	"github.com/gorilla/mux"
)

type tagRequest struct {
	Name string `json:"name"`
}

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
func (s *server) createTag(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.Logger.Debugf("create tag: read request body error: %v", err)
		s.Logger.Error("create tag: read request body error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	err = json.Unmarshal(body, &tagr)
	if err != nil {
		s.Logger.Debugf("create tag: unmarshal tag name error: %v", err)
		s.Logger.Errorf("create tag: unmarshal tag name error")
		jsonhttp.InternalServerError(w, "error unmarshaling metadata")
		return
	}

	tag, err := s.Tags.Create(tagr.Name, 0, false)
	if err != nil {
		s.Logger.Debugf("bzz-tag: tag create error: %v", err)
		s.Logger.Error("bzz-tag: tag create error")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.Created(w, newTagResponse(tag))

}

func (s *server) getTag(w http.ResponseWriter, r *http.Request) {
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
