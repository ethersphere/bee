// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
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

func (s *Service) createTagHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_tag").Build()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		logger.Debug("read request body failed", "error", err)
		logger.Error(nil, "read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			logger.Debug("unmarshal tag name failed", "error", err)
			logger.Error(nil, "unmarshal tag name failed")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.tags.Create(0)
	if err != nil {
		logger.Debug("create tag failed", "error", err)
		logger.Error(nil, "create tag failed")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.Created(w, newTagResponse(tag))
}

func (s *Service) getTagHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_tag").Build()

	paths := struct {
		TagID uint32 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	tag, err := s.tags.Get(paths.TagID)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			logger.Debug("tag not found", "tag_id", paths.TagID)
			logger.Error(nil, "tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		logger.Debug("get tag failed", "tag_id", paths.TagID, "error", err)
		logger.Error(nil, "get tag failed", "tag_id", paths.TagID)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}

func (s *Service) deleteTagHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("delete_tag").Build()

	paths := struct {
		TagID uint32 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	tag, err := s.tags.Get(paths.TagID)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			logger.Debug("tag not found", "tag_id", paths.TagID)
			logger.Error(nil, "tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		logger.Debug("get tag failed", "tag_id", paths.TagID, "error", err)
		logger.Error(nil, "get tag failed", "tag_id", paths.TagID)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	s.tags.Delete(tag.Uid)
	jsonhttp.NoContent(w)
}

func (s *Service) doneSplitHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("patch_tag").Build()

	paths := struct {
		TagID uint32 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		logger.Debug("read request body failed", "error", err)
		logger.Error(nil, "read request body failed")
		jsonhttp.InternalServerError(w, "cannot read request")
		return
	}

	tagr := tagRequest{}
	if len(body) > 0 {
		err = json.Unmarshal(body, &tagr)
		if err != nil {
			logger.Debug("unmarshal tag name failed", "error", err)
			logger.Error(nil, "unmarshal tag name failed")
			jsonhttp.InternalServerError(w, "error unmarshaling metadata")
			return
		}
	}

	tag, err := s.tags.Get(paths.TagID)
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			logger.Debug("tag not found", "tag_id", paths.TagID)
			logger.Error(nil, "tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		logger.Debug("get tag failed", "tag_id", paths.TagID, "error", err)
		logger.Error(nil, "get tag failed", "tag_id", paths.TagID)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	_, err = tag.DoneSplit(tagr.Address)
	if err != nil {
		logger.Debug("done split failed", "address", tagr.Address, "error", err)
		logger.Error(nil, "done split failed", "address", tagr.Address)
		jsonhttp.InternalServerError(w, "done split: failed")
		return
	}
	jsonhttp.OK(w, "ok")
}

func (s *Service) listTagsHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_tags").Build()

	queries := struct {
		Offset int `map:"offset"`
		Limit  int `map:"limit"`
	}{
		Limit: 100, // Default limit.
	}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	tagList, err := s.tags.ListAll(queries.Offset, queries.Limit)
	if err != nil {
		logger.Debug("listing failed", "offset", queries.Offset, "limit", queries.Limit, "error", err)
		logger.Error(nil, "listing failed")
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
