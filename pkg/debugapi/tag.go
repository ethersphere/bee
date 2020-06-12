// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"errors"
	"fmt"
	"math/rand"
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

func (s *server) createTag(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = fmt.Sprintf("tag-%v-%v", time.Now().UnixNano(), rand.Int())
	}

	tag, err := s.Tags.Create(name, 0, false)
	if err != nil {
		s.Logger.Debugf("create tag: %s %v", name, err)
		s.Logger.Errorf("create tag: %s error", name)
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))

}

func (s *server) getTag(w http.ResponseWriter, r *http.Request) {
	uidStr := mux.Vars(r)["uid"]

	uid, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		s.Logger.Debugf("get tag: parse uid  %s: %v", uidStr, err)
		s.Logger.Error("get tag: parse uid")
		jsonhttp.BadRequest(w, "invalid uid")
		return
	}

	tag, err := s.Tags.Get(uint32(uid))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.Logger.Debugf("get tag: tag %v not present: %v", uid, err)
			s.Logger.Warningf("get tag: tag %v not present", uid)
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.Logger.Debugf("get tag: tag %v: %v", uid, err)
		s.Logger.Errorf("get tag: %v", uid)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newTagResponse(tag))
}
