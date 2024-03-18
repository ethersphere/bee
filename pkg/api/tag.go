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

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type tagRequest struct {
	Address swarm.Address `json:"address,omitempty"`
}

type tagResponse struct {
	Split     uint64        `json:"split"`
	Seen      uint64        `json:"seen"`
	Stored    uint64        `json:"stored"`
	Sent      uint64        `json:"sent"`
	Synced    uint64        `json:"synced"`
	Uid       uint64        `json:"uid"`
	Address   swarm.Address `json:"address"`
	StartedAt time.Time     `json:"startedAt"`
}

func newTagResponse(tag storer.SessionInfo) tagResponse {
	return tagResponse{
		Split:     tag.Split,
		Seen:      tag.Seen,
		Stored:    tag.Stored,
		Sent:      tag.Sent,
		Synced:    tag.Synced,
		Uid:       tag.TagID,
		Address:   tag.Address,
		StartedAt: time.Unix(0, tag.StartedAt),
	}
}

type listTagsResponse struct {
	Tags []tagResponse `json:"tags"`
}

func (s *Service) createTagHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_tag").Build()

	tag, err := s.storer.NewSession()
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
		TagID uint64 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	tag, err := s.storer.Session(paths.TagID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
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
		TagID uint64 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	if err := s.storer.DeleteSession(paths.TagID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
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

	jsonhttp.NoContent(w)
}

func (s *Service) doneSplitHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("patch_tag").Build()

	paths := struct {
		TagID uint64 `map:"id" validate:"required"`
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

	tag, err := s.storer.Session(paths.TagID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
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

	putter, err := s.storer.Upload(r.Context(), false, tag.TagID)
	if err != nil {
		logger.Debug("get tag failed", "tag_id", paths.TagID, "error", err)
		logger.Error(nil, "get tag failed", "tag_id", paths.TagID)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	err = putter.Done(tagr.Address)
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

	tagList, err := s.storer.ListSessions(queries.Offset, queries.Limit)
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
