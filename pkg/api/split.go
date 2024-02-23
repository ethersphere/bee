// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"net/http"

	"github.com/ethersphere/bee/pkg/file/redundancy"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

type splitPutter struct {
	chunks []swarm.Chunk
}

func (s *splitPutter) Put(_ context.Context, chunk swarm.Chunk) error {
	s.chunks = append(s.chunks, chunk)
	return nil
}

func (s *Service) splitHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("split").Build()
	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error(nil, "streaming unsupported")
		jsonhttp.InternalServerError(w, "streaming unsupported")
		return
	}

	headers := struct {
		RLevel redundancy.Level `map:"Swarm-Redundancy-Level"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}

	putter := &splitPutter{
		chunks: make([]swarm.Chunk, 0),
	}

	p := requestPipelineFn(putter, false, headers.RLevel)
	_, err := p(r.Context(), r.Body)
	if err != nil {
		logger.Error(err, "split: pipeline error")
		jsonhttp.InternalServerError(w, "split failed")
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")

	buf := make([]byte, swarm.HashSize+swarm.ChunkSize)
	for _, c := range putter.chunks {
		copy(buf[:swarm.HashSize], c.Address().Bytes())
		copy(buf[swarm.HashSize:], c.Data())
		_, err = w.Write(buf)
		if err != nil {
			logger.Error(err, "split: write error")
			return
		}
		flusher.Flush()
	}
}
