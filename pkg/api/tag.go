// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
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

func (s *Service) createTagHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debug("create tag: read request body failed", "error", err)
		s.logger.Error(nil, "create tag: read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			s.logger.Debug("create tag: unmarshal tag name failed", "error", err)
			s.logger.Error(nil, "create tag: unmarshal tag name failed")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.tags.Create(0)
	if err != nil {
		s.logger.Debug("create tag: create tag failed", "error", err)
		s.logger.Error(nil, "create tag: create tag failed")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.Created(w, newTagResponse(tag))
}

func (s *Service) getTagHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Id uint32 `parse:"id" name:"id"`
	}{}
	err := s.parseAndValidate(mux.Vars(r), &path)
	if err != nil {
		s.logger.Debug("get tag: parse id string failed", "string", mux.Vars(r)["id"], "error", err)
		s.logger.Error(nil, "get tag: parse id string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	tag, err := s.tags.Get(path.Id)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debug("get tag: tag not found", "tag_id", path.Id)
			s.logger.Error(nil, "get tag: tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debug("get tag: get tag failed", "tag_id", path.Id, "error", err)
		s.logger.Error(nil, "get tag: get tag failed", "tag_id", path.Id)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}

func (s *Service) deleteTagHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Id uint32 `parse:"id" name:"id"`
	}{}
	err := s.parseAndValidate(mux.Vars(r), &path)

	if err != nil {
		s.logger.Debug("delete tag: parse id string failed", "string", mux.Vars(r)["id"], "error", err)
		s.logger.Error(nil, "delete tag: parse id string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	tag, err := s.tags.Get(path.Id)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debug("delete tag: tag not found", "tag_id", path.Id)
			s.logger.Error(nil, "delete tag: tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debug("delete tag: get tag failed", "tag_id", path.Id, "error", err)
		s.logger.Error(nil, "delete tag: get tag failed", "tag_id", path.Id)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	s.tags.Delete(tag.Uid)
	jsonhttp.NoContent(w)
}

func (s *Service) doneSplitHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Id uint32 `parse:"id" name:"id"`
	}{}
  err := s.parseAndValidate(mux.Vars(r), &path)
	if err != nil {
		s.logger.Debug("done split tag: parse id string failed", "string", mux.Vars(r)["id"], "error", err)
		s.logger.Error(nil, "done split tag: parse id string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debug("done split tag: read request body failed", "error", err)
		s.logger.Error(nil, "done split tag: read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			s.logger.Debug("done split tag: unmarshal tag name failed", "error", err)
			s.logger.Error(nil, "done split tag: unmarshal tag name failed")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.tags.Get(path.Id)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debug("done split tag: tag not found", "tag_id", path.Id)
			s.logger.Error(nil, "done split tag: tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debug("done split tag: get tag failed", "tag_id", path.Id, "error", err)
		s.logger.Error(nil, "done split tag: get tag failed", "tag_id", path.Id)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	_, err = tag.DoneSplit(tagr.Address)
	if err != nil {
		s.logger.Debug("done split tag: done split failed", "address", tagr.Address, "error", err)
		s.logger.Error(nil, "done split tag: done split failed", "address", tagr.Address)
		jsonhttp.InternalServerError(w, "done split: failed")
		return
	}
	jsonhttp.OK(w, "ok")
}

func (s *Service) listTagsHandler(w http.ResponseWriter, r *http.Request) {
	var (
		err           error
		offset, limit = 0, 100 // default offset is 0, default limit 100
	)

	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			s.logger.Debug("list tags: parse offset string failed", "string", v, "error", err)
			s.logger.Error(nil, "list tags: parse offset string failed")
			jsonhttp.BadRequest(w, "bad offset")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			s.logger.Debug("list tags: parse limit string failed", "string", v, "error", err)
			s.logger.Error(nil, "list tags: parse limit string failed")
			jsonhttp.BadRequest(w, "bad limit")
		}
	}

	tagList, err := s.tags.ListAll(r.Context(), offset, limit)
	if err != nil {
		s.logger.Debug("list tags: listing failed", "offset", offset, "limit", limit, "error", err)
		s.logger.Error(nil, "list tags: listing failed")
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
