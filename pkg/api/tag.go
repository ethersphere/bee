// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
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
	Name    string        `json:"name"`
	Address swarm.Address `json:"address"`
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

	if tagr.Name == "" {
		tagr.Name = fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
	}

	tag, err := s.Tags.Create(tagr.Name, 0, false)
	if err != nil {
		s.Logger.Debugf("create tag: tag create error: %v", err)
		s.Logger.Error("create tag: tag create error")
		jsonhttp.InternalServerError(w, "cannot create tag")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.Created(w, newTagResponse(tag))
}

func (s *server) getTag(w http.ResponseWriter, r *http.Request) {
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

func (s *server) deleteTag(w http.ResponseWriter, r *http.Request) {
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
			s.Logger.Error("delete tag: tag not present", id)
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.Logger.Debugf("delete tag: tag %v: %v", idStr, err)
		s.Logger.Errorf("delete tag: %v", idStr)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	s.Tags.Delete(tag.Uid)
	jsonhttp.NoContent(w, "ok")
}

func (s *server) doneSplit(w http.ResponseWriter, r *http.Request) {
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
	err = json.Unmarshal(body, &tagr)
	if err != nil {
		s.Logger.Debugf("done split tag: unmarshal tag name error: %v", err)
		s.Logger.Errorf("done split tag: unmarshal tag name error")
		jsonhttp.InternalServerError(w, "error unmarshaling metadata")
		return
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

	tag.DoneSplit(tagr.Address)
	jsonhttp.Respond(w, http.StatusNoContent, nil)
}
