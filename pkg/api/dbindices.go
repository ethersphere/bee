// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type StorageIndexDebugger interface {
	DebugIndices() (map[string]int, error)
}

func (s *Service) dbIndicesHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("db_indices").Build()

	if s.indexDebugger == nil {
		jsonhttp.NotImplemented(w, "storage indices not available")
		logger.Error(nil, "db indices not implemented")
		return
	}

	indices, err := s.indexDebugger.DebugIndices()
	if err != nil {
		jsonhttp.InternalServerError(w, "cannot get storage indices")
		logger.Debug("db indices failed", "error", err)
		logger.Error(nil, "db indices failed")
		return
	}

	jsonhttp.OK(w, indices)
}
