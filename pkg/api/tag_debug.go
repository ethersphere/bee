// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	storer "github.com/ethersphere/bee/pkg/localstorev2"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type debugTagResponse struct {
	Split     uint64        `json:"split"`
	Seen      uint64        `json:"seen"`
	Stored    uint64        `json:"stored"`
	Sent      uint64        `json:"sent"`
	Synced    uint64        `json:"synced"`
	Uid       uint64        `json:"uid"`
	Address   swarm.Address `json:"address"`
	StartedAt time.Time     `json:"startedAt"`
}

func newDebugTagResponse(tag storer.SessionInfo) debugTagResponse {
	return debugTagResponse{
		Split:     tag.Split,
		Seen:      tag.Seen,
		Stored:    tag.Stored,
		Sent:      tag.Sent,
		Synced:    tag.Synced,
		Uid:       tag.TagID,
		Address:   tag.Address,
		StartedAt: time.Unix(tag.StartedAt, 0),
	}
}

func (s *Service) getDebugTagHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_tag").Build()

	paths := struct {
		TagID uint64 `map:"id" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	tag, err := s.storer.GetSessionInfo(paths.TagID)
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
	jsonhttp.OK(w, newDebugTagResponse(tag))
}
