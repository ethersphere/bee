// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/gorilla/mux"
)

type debugTagResponse struct {
	Total     int64         `json:"total"`
	Split     int64         `json:"split"`
	Seen      int64         `json:"seen"`
	Stored    int64         `json:"stored"`
	Sent      int64         `json:"sent"`
	Synced    int64         `json:"synced"`
	Uid       uint32        `json:"uid"`
	Address   swarm.Address `json:"address"`
	StartedAt time.Time     `json:"startedAt"`
}

func newDebugTagResponse(tag *tags.Tag) debugTagResponse {
	return debugTagResponse{
		Total:     tag.Total,
		Split:     tag.Split,
		Seen:      tag.Seen,
		Stored:    tag.Stored,
		Sent:      tag.Sent,
		Synced:    tag.Synced,
		Uid:       tag.Uid,
		Address:   tag.Address,
		StartedAt: tag.StartedAt,
	}
}

func (s *Service) getDebugTagHandler(w http.ResponseWriter, r *http.Request) {
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
	jsonhttp.OK(w, newDebugTagResponse(tag))
}
