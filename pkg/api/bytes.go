// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/gorilla/mux"
	"github.com/opentracing/opentracing-go/ext"
	olog "github.com/opentracing/opentracing-go/log"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *Service) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	span, logger, ctx := s.tracer.StartSpanFromContext(r.Context(), "post_bytes", s.logger.WithName("post_bytes").Build())
	defer span.Finish()

	headers := struct {
		BatchID        []byte           `map:"Swarm-Postage-Batch-Id" validate:"required"`
		SwarmTag       uint64           `map:"Swarm-Tag"`
		Pin            bool             `map:"Swarm-Pin"`
		Deferred       *bool            `map:"Swarm-Deferred-Upload"`
		Encrypt        bool             `map:"Swarm-Encrypt"`
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
		logger.Debug("get putter failed", "error", err)
		logger.Error(nil, "get putter failed")
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

	p := requestPipelineFn(putter, headers.Encrypt, headers.RLevel)
	reference, err := p(ctx, r.Body)
	if err != nil {
		logger.Debug("split write all failed", "error", err)
		logger.Error(nil, "split write all failed")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(ow, "batch is overissued")
		default:
			jsonhttp.InternalServerError(ow, "split write all failed")
		}
		ext.LogError(span, err, olog.String("action", "split.WriteAll"))
		return
	}

	encryptedReference := reference
	if headers.Act {
		encryptedReference, err = s.actEncryptionHandler(r.Context(), logger, w, putter, reference, headers.HistoryAddress)
		if err != nil {
			jsonhttp.InternalServerError(w, errActUpload)
			return
		}
	}
	// TODO: what should be the root_address ? (eref vs ref)
	span.SetTag("root_address", reference)

	err = putter.Done(reference)
	if err != nil {
		logger.Debug("done split failed", "error", err)
		logger.Error(nil, "done split failed")
		jsonhttp.InternalServerError(ow, "done split failed")
		ext.LogError(span, err, olog.String("action", "putter.Done"))
		return
	}

	if tag != 0 {
		w.Header().Set(SwarmTagHeader, fmt.Sprint(tag))
	}

	span.LogFields(olog.Bool("success", true))

	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, bytesPostResponse{
		Reference: encryptedReference,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *Service) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_bytes_by_address").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	additionalHeaders := http.Header{
		ContentTypeHeader: {"application/octet-stream"},
	}

	s.downloadHandler(logger, w, r, address, additionalHeaders, true, false)
}

func (s *Service) bytesHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("head_bytes_by_address").Build())

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.IsZero() {
		address = v
	}

	getter := s.storer.Download(true)

	ch, err := getter.Get(r.Context(), address)
	if err != nil {
		logger.Debug("get root chunk failed", "chunk_address", address, "error", err)
		logger.Error(nil, "get rook chunk failed")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Add("Access-Control-Expose-Headers", "Accept-Ranges, Content-Encoding")
	w.Header().Add(ContentTypeHeader, "application/octet-stream")
	var span int64

	if cac.Valid(ch) {
		span = int64(binary.LittleEndian.Uint64(ch.Data()[:swarm.SpanSize]))
	} else {
		// soc
		span = int64(len(ch.Data()))
	}
	w.Header().Set(ContentLengthHeader, strconv.FormatInt(span, 10))
	w.WriteHeader(http.StatusOK) // HEAD requests do not write a body
}
