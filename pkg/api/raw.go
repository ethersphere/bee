// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type rawPostResponse struct {
	Hash swarm.Address `json:"hash"`
}

// rawUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) rawUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	responseObject, err := s.splitUpload(ctx, r.Body, r.ContentLength)
	if err != nil {
		s.Logger.Debugf("raw: %v", err)
		o := responseObject.(jsonhttp.StatusResponse)
		jsonhttp.Respond(w, o.Code, o)
	} else {
		jsonhttp.OK(w, responseObject)
	}
}

func (s *server) splitUpload(ctx context.Context, r io.ReadCloser, l int64) (interface{}, error) {
	chunkPipe := file.NewChunkPipe()
	go func() {
		buf := make([]byte, swarm.ChunkSize)
		c, err := io.CopyBuffer(chunkPipe, r, buf)
		if err != nil {
			s.Logger.Debugf("split upload: io error %d: %v", c, err)
			s.Logger.Error("io error")
			return
		}
		if c != l {
			s.Logger.Debugf("split upload: read count mismatch %d: %v", c, err)
			s.Logger.Error("read count mismatch")
			return
		}
		err = chunkPipe.Close()
		if err != nil {
			s.Logger.Errorf("split upload: incomplete file write close %v", err)
			s.Logger.Error("incomplete file write close")
		}
	}()
	sp := splitter.NewSimpleSplitter(s.Storer)
	address, err := sp.Split(ctx, chunkPipe, l)
	var response jsonhttp.StatusResponse
	if err != nil {
		response.Message = "upload error"
		response.Code = http.StatusInternalServerError
		err = fmt.Errorf("%s: %v", response.Message, err)
		return response, err
	}
	return rawPostResponse{Hash: address}, nil
}

// rawGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *server) rawGetHandler(w http.ResponseWriter, r *http.Request) {
	addressHex := mux.Vars(r)["address"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(addressHex)
	if err != nil {
		s.Logger.Debugf("raw: parse address %s: %v", addressHex, err)
		s.Logger.Error("raw: parse address error")
		jsonhttp.BadRequest(w, "invalid address")
		return
	}

	j := joiner.NewSimpleJoiner(s.Storer)

	dataSize, err := j.Size(ctx, address)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			s.Logger.Debugf("raw: not found %s: %v", address, err)
			s.Logger.Error("raw: not found")
			jsonhttp.NotFound(w, "not found")
			return
		}
		s.Logger.Debugf("raw: invalid root chunk %s: %v", address, err)
		s.Logger.Error("raw: invalid root chunk")
		jsonhttp.BadRequest(w, "invalid root chunk")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", dataSize))
	c, err := file.JoinReadAll(j, address, w)
	if err != nil && c == 0 {
		s.Logger.Errorf("raw: data write %s: %v", address, err)
		s.Logger.Error("raw: data input error")
		jsonhttp.InternalServerError(w, "retrieval fail")
	}
}
