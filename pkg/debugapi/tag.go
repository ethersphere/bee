// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
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
		Address:   tag.Address,
		StartedAt: tag.StartedAt,
	}
}

func (s *Service) getTagHandler(w http.ResponseWriter, r *http.Request) {
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
