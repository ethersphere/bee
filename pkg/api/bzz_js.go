// go:build js
//go:build js
// +build js

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
)

func (s *Service) bzzUploadHandler(w http.ResponseWriter, r *http.Request) {
	span, logger, ctx := s.tracer.StartSpanFromContext(r.Context(), "post_bzz", s.logger.WithName("post_bzz").Build())
	defer span.Finish()

	headers := struct {
		ContentType    string           `map:"Content-Type,mimeMediaType" validate:"required"`
		BatchID        []byte           `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64           `map:"Swarm-Tag"`
		Pin            bool             `map:"Swarm-Pin"`
		Deferred       *bool            `map:"Swarm-Deferred-Upload"`
		Encrypt        bool             `map:"Swarm-Encrypt"`
		IsDir          bool             `map:"Swarm-Collection"`
		RLevel         redundancy.Level `map:"Swarm-Redundancy-Level"`
		Act            bool             `map:"Swarm-Act"`
		HistoryAddress swarm.Address    `map:"Swarm-Act-History-Address"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	var (
		tag      uint64
		err      error
		deferred = defaultUploadMethod(headers.Deferred)
	)

	if deferred || headers.Pin {
		tag, err = s.getOrCreateSessionID(headers.SwarmTag)
		if err != nil {
			logger.Debug("get or create tag failed", "error", err)
			logger.Error(nil, "get or create tag failed")
			switch {
			case errors.Is(err, storage.ErrNotFound):
				jsonhttp.NotFound(w, "tag not found")
			default:
				jsonhttp.InternalServerError(w, "cannot get or create tag")
			}
			ext.LogError(span, err, olog.String("action", "tag.create"))
			return
		}
		span.SetTag("tagID", tag)
	}

	putter, err := s.newStamperPutter(ctx, putterOptions{
		BatchID:  headers.BatchID,
		TagID:    tag,
		Pin:      headers.Pin,
		Deferred: deferred,
	})
	if err != nil {
		logger.Debug("putter failed", "error", err)
		logger.Error(nil, "putter failed")
		switch {
		case errors.Is(err, errBatchUnusable) || errors.Is(err, postage.ErrNotUsable):
			jsonhttp.UnprocessableEntity(w, "batch not usable yet or does not exist")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch with id not found")
		case errors.Is(err, errInvalidPostageBatch):
			jsonhttp.BadRequest(w, "invalid batch id")
		case errors.Is(err, errUnsupportedDevNodeOperation):
			jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		default:
			jsonhttp.BadRequest(w, nil)
		}
		ext.LogError(span, err, olog.String("action", "new.StamperPutter"))
		return
	}

	ow := &cleanupOnErrWriter{
		ResponseWriter: w,
		onErr:          putter.Cleanup,
		logger:         logger,
	}

	if headers.IsDir || headers.ContentType == multiPartFormData {
		s.dirUploadHandler(ctx, logger, span, ow, r, putter, r.Header.Get(ContentTypeHeader), headers.Encrypt, tag, headers.RLevel, headers.Act, headers.HistoryAddress)
		return
	}
	s.fileUploadHandler(ctx, logger, span, ow, r, putter, headers.Encrypt, tag, headers.RLevel, headers.Act, headers.HistoryAddress)
}
