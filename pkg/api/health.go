// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type statusResponse struct {
	Status          string `json:"status"`
	Version         string `json:"version"`
	APIVersion      string `json:"apiVersion"`
	DebugAPIVersion string `json:"debugApiVersion"`
}

func (s *Service) healthHandler(w http.ResponseWriter, r *http.Request) {
	status := s.probe.Healthy()
	jsonhttp.OK(w, statusResponse{
		Status:          status.String(),
		Version:         bee.Version,
		APIVersion:      Version,
		DebugAPIVersion: Version,
	})
}
