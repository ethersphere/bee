package api

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
	idStr := mux.Vars(r)["id"]

	id, err := strconv.Atoi(idStr)
	if err != nil {
		s.logger.Debug("get tag: parse id string failed", "string", idStr, "error", err)
		s.logger.Error(nil, "get tag: parse id string failed")
		jsonhttp.BadRequest(w, "invalid id")
		return
	}

	tag, err := s.tags.Get(uint32(id))
	if err != nil {
		if errors.Is(err, tags.ErrNotFound) {
			s.logger.Debug("get tag: tag not found", "tag_id", id)
			s.logger.Error(nil, "get tag: tag not found")
			jsonhttp.NotFound(w, "tag not present")
			return
		}
		s.logger.Debug("get tag: get tag failed", "tag_id", id, "error", err)
		s.logger.Error(nil, "get tag: get tag failed", "tag_id", id)
		jsonhttp.InternalServerError(w, "cannot get tag")
		return
	}

	w.Header().Set("Cache-Control", "no-cache, private, max-age=0")
	jsonhttp.OK(w, newDebugTagResponse(tag))
}
