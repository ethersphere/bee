// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
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
	Total     int64     `json:"total"`
	Processed int64     `json:"processed"`
	Synced    int64     `json:"synced"`
}

type listTagsResponse struct {
	Tags []tagResponse `json:"tags"`
}

func newTagResponse(tag *tags.Tag) tagResponse {
	return tagResponse{
		Uid:       tag.Uid,
		StartedAt: tag.StartedAt,
		Total:     tag.Total,
		Processed: tag.Stored,
		Synced:    tag.Seen + tag.Synced,
	}
}

func (s *server) createTagHandler(w http.ResponseWriter, r *http.Request) {
	tagr := tagRequest{}

	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&tagr); err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("create tag: read request body error: %v", err)
		s.logger.Error("create tag: read request body error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tag, err := s.tags.Create(0)
	if err != nil {
		s.logger.Debugf("create tag: tag create error: %v", err)
		s.logger.Error("create tag: tag create error")
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
		s.logger.Debugf("get tag: parse id  %s: %v", idStr, err)
		s.logger.Error("get tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tag, err := s.tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debugf("get tag: tag not present: %v, id %s", err, idStr)
			s.logger.Error("get tag: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debugf("get tag: tag %v: %v", idStr, err)
		s.logger.Errorf("get tag: %v", idStr)
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
		s.logger.Debugf("delete tag: parse id  %s: %v", idStr, err)
		s.logger.Error("delete tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tag, err := s.tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debugf("delete tag: tag not present: %v, id %s", err, idStr)
			s.logger.Error("delete tag: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debugf("delete tag: tag %v: %v", idStr, err)
		s.logger.Errorf("delete tag: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	s.tags.Delete(tag.Uid)
	jsonhttp.NoContent(w)
}

func (s *server) doneSplitHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.logger.Debugf("done split tag: parse id  %s: %v", idStr, err)
		s.logger.Error("done split tag: parse id")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tagr := tagRequest{}

	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(&tagr); err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("done split tag: read request body error: %v", err)
		s.logger.Error("done split tag: read request body error")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tag, err := s.tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debugf("done split: tag not present: %v, id %s", err, idStr)
			s.logger.Error("done split: tag not present")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debugf("done split: tag %v: %v", idStr, err)
		s.logger.Errorf("done split: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	_, err = tag.DoneSplit(tagr.Address)
	if err != nil {
		s.logger.Debugf("done split: failed for address %v", tagr.Address)
		s.logger.Errorf("done split: failed for address %v", tagr.Address)
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
			s.logger.Debugf("list tags: parse offset: %v", err)
			s.logger.Errorf("list tags: bad offset")
			jsonhttp.BadRequest(w, "bad offset")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			s.logger.Debugf("list tags: parse limit: %v", err)
			s.logger.Errorf("list tags: bad limit")
			jsonhttp.BadRequest(w, "bad limit")
		}
	}

	tagList, err := s.tags.ListAll(r.Context(), offset, limit)
	if err != nil {
		s.logger.Debugf("list tags: listing: %v", err)
		s.logger.Errorf("list tags: listing")
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
