// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
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
	Address swarm.Address `json:"address,omitempty"`
}

type tagResponse struct {
	Uid       uint32    `json:"uid"`
	StartedAt time.Time `json:"startedAt"`
	Stored    int64     `json:"stored"`
	Synced    int64     `json:"synced"`
}

type listTagsResponse struct {
	Tags []tagResponse `json:"tags"`
}

func newTagResponse(tag *tags.Tag) tagResponse {
	return tagResponse{
		Uid:       tag.Uid,
		StartedAt: tag.StartedAt,
		Stored:    tag.Stored,
		Synced:    tag.Seen + tag.Synced,
	}
}

func (s *server) createTagHandler(w http.ResponseWriter, r *http.Request) {
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
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			s.Logger.Debugf("create tag: unmarshal tag name error: %v", err)
			s.Logger.Errorf("create tag: unmarshal tag name error")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.Tags.Create(0)
	if err != nil {
		s.Logger.Debugf("create tag: tag create error: %v", err)
		s.Logger.Error("create tag: tag create error")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.Created(w, newTagResponse(tag))
}

func (s *server) getTagHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.Logger.Debugf("get tag: parse id  %s: %v", idStr, err)
		s.Logger.Error("get tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tag, err := s.Tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.Logger.Debugf("get tag: tag not present: %v, id %s", err, idStr)
			s.Logger.Error("get tag: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.Logger.Debugf("get tag: tag %v: %v", idStr, err)
		s.Logger.Errorf("get tag: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}

func (s *server) deleteTagHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.Logger.Debugf("delete tag: parse id  %s: %v", idStr, err)
		s.Logger.Error("delete tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tag, err := s.Tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.Logger.Debugf("delete tag: tag not present: %v, id %s", err, idStr)
			s.Logger.Error("delete tag: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.Logger.Debugf("delete tag: tag %v: %v", idStr, err)
		s.Logger.Errorf("delete tag: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	s.Tags.Delete(tag.Uid)
	jsonhttp.NoContent(w)
}

func (s *server) doneSplitHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.Logger.Debugf("done split tag: parse id  %s: %v", idStr, err)
		s.Logger.Error("done split tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.Logger.Debugf("done split tag: read request body error: %v", err)
		s.Logger.Error("done split tag: read request body error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			s.Logger.Debugf("done split tag: unmarshal tag name error: %v", err)
			s.Logger.Errorf("done split tag: unmarshal tag name error")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.Tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.Logger.Debugf("done split: tag not present: %v, id %s", err, idStr)
			s.Logger.Error("done split: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.Logger.Debugf("done split: tag %v: %v", idStr, err)
		s.Logger.Errorf("done split: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	_, err = tag.DoneSplit(tagr.Address)
	if err != nil {
		s.Logger.Debugf("done split: failed for address %v", tagr.Address)
		s.Logger.Errorf("done split: failed for address %v", tagr.Address)
		jsonhttp.InternalServerError(w, nil)
		return
	}
	jsonhttp.OK(w, "ok")
}

func (s *server) listTagsHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err           error
		offset, limit = 0, 100 // default offset is 0, default limit 100
	)

	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			s.Logger.Debugf("list tags: parse offset: %v", err)
			s.Logger.Errorf("list tags: bad offset")
			jsonhttp.BadRequest(w, "bad offset")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			s.Logger.Debugf("list tags: parse limit: %v", err)
			s.Logger.Errorf("list tags: bad limit")
			jsonhttp.BadRequest(w, "bad limit")
		}
	}

	tagList, err := s.Tags.List(r.Context(), offset, limit)
	if err != nil {
		s.Logger.Debugf("list tags: listing: %v", err)
		s.Logger.Errorf("list tags: listing")
		jsonhttp.InternalServerError(w, err)
		return
	}

	tags := make([]tagResponse, len(tagList))
	for i, t := range tagList {
		tags[i] = newTagResponse(t)
	}

	jsonhttp.OK(w, listTagsResponse{
		Tags: tags,
	})
}
