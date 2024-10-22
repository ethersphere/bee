// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

type ReadyStatusResponse healthStatusResponse

func (s *Service) readinessHandler(w http.ResponseWriter, _ *http.Request) {
	if s.probe.Ready() == ProbeStatusOK {
		jsonhttp.OK(w, ReadyStatusResponse{
			Status:     "ready",
			Version:    bee.Version,
			APIVersion: Version,
		})
	} else {
		jsonhttp.BadRequest(w, ReadyStatusResponse{
			Status:     "notReady",
			Version:    bee.Version,
			APIVersion: Version,
		})
	}
}
