// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/util/ioutil"
	"github.com/gorilla/mux"
)

type bytesPostResponse struct {
	Reference swarm.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *Service) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)

	putter, wait, err := s.newStamperPutter(r)
	if err != nil {
		logger.Debugf("bytes upload: get putter:%v", err)
		logger.Error("bytes upload: putter")
		jsonhttp.BadRequest(w, nil)
		return
	}

	if strings.Contains(strings.ToLower(r.Header.Get(contentTypeHeader)), "multipart/form-data") {
		logger.Error("bytes upload: multipart uploads are not supported on this endpoint")
		jsonhttp.BadRequest(w, nil)
		return
	}

	tag, created, err := s.getOrCreateTag(r.Header.Get(SwarmTagHeader))
	if err != nil {
		logger.Debugf("bytes upload: get or create tag: %v", err)
		logger.Error("bytes upload: get or create tag")
		jsonhttp.InternalServerError(w, "cannot get or create tag")
		return
	}

	if !created {
		// only in the case when tag is sent via header (i.e. not created by this request)
		if estimatedTotalChunks := requestCalculateNumberOfChunks(r); estimatedTotalChunks > 0 {
			err = tag.IncN(tags.TotalChunks, estimatedTotalChunks)
			if err != nil {
				s.logger.Debugf("bytes upload: increment tag: %v", err)
				s.logger.Error("bytes upload: increment tag")
				jsonhttp.InternalServerError(w, "increment tag")
				return
			}
		}
	}

	// Add the tag to the context
	ctx := sctx.SetTag(r.Context(), tag)
	p := requestPipelineFn(putter, r)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	pr := ioutil.TimeoutReader(ctx, r.Body, time.Minute, func(n uint64) {
		logger.Error("bytes upload: idle read timeout exceeded")
		logger.Debugf("bytes upload: idle read timeout exceeded: read %d", n)
		cancel()
	})
	address, err := p(ctx, pr)
	if err != nil {
		logger.Debugf("bytes upload: split write all: %v", err)
		logger.Error("bytes upload: split write all")
		switch {
		case errors.Is(err, postage.ErrBucketFull):
			jsonhttp.PaymentRequired(w, "batch is overissued")
		default:
			jsonhttp.InternalServerError(w, nil)
		}
		return
	}
	if err = wait(); err != nil {
		logger.Debugf("bytes upload: sync chunks: %v", err)
		logger.Error("bytes upload: sync chunks")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	if created {
		_, err = tag.DoneSplit(address)
		if err != nil {
			logger.Debugf("bytes upload: done split: %v", err)
			logger.Error("bytes upload: done split failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}

	if strings.ToLower(r.Header.Get(SwarmPinHeader)) == "true" {
		if err := s.pinning.CreatePin(ctx, address, false); err != nil {
			logger.Debugf("bytes upload: creation of pin for %q failed: %v", address, err)
			logger.Error("bytes upload: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}

	w.Header().Set(SwarmTagHeader, fmt.Sprint(tag.Uid))
	w.Header().Set("Access-Control-Expose-Headers", SwarmTagHeader)
	jsonhttp.Created(w, bytesPostResponse{
		Reference: address,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *Service) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debugf("bytes: parse address %s: %v", nameOrHex, err)
		logger.Error("bytes: parse address error")
		jsonhttp.NotFound(w, nil)
		return
	}

	additionalHeaders := http.Header{
		"Content-Type": {"application/octet-stream"},
	}

	s.downloadHandler(w, r, address, additionalHeaders, true)
}

func (s *Service) bytesHeadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	nameOrHex := mux.Vars(r)["address"]

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debugf("bytes: parse address %s: %v", nameOrHex, err)
		logger.Error("bytes: parse address error")
		w.WriteHeader(http.StatusBadRequest) // HEAD requests do not write a body
		return
	}
	ch, err := s.storer.Get(r.Context(), storage.ModeGetRequest, address)
	if err != nil {
		logger.Debugf("bytes: get root chunk %s: %v", nameOrHex, err)
		logger.Error("bytes: get rook chunk error")
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Add("Access-Control-Expose-Headers", "Accept-Ranges, Content-Encoding")
	w.Header().Add("Content-Type", "application/octet-stream")
	var span int64

	if cac.Valid(ch) {
		span = int64(binary.LittleEndian.Uint64(ch.Data()[:swarm.SpanSize]))
	} else {
		// soc
		span = int64(len(ch.Data()))
	}
	w.Header().Add("Content-Length", fmt.Sprintf("%d", span))
	w.WriteHeader(http.StatusOK) // HEAD requests do not write a body
}
