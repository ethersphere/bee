// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

type healthStatusResponse struct {
	Status     string `json:"status"`
	Version    string `json:"version"`
	APIVersion string `json:"apiVersion"`
}

func (s *Service) healthHandler(w http.ResponseWriter, _ *http.Request) {
	status := s.probe.Healthy()
	jsonhttp.OK(w, healthStatusResponse{
		Status:     status.String(),
		Version:    bee.Version,
		APIVersion: Version,
	})
}
