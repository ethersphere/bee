// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

func (s *Service) topologyHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_topology").Build()

	params := s.topologyDriver.Snapshot()

	params.LightNodes = s.lightNodes.PeerInfo()

	b, err := json.Marshal(params)
	if err != nil {
		logger.Error(err, "marshal to json failed")
		jsonhttp.InternalServerError(w, err)
		return
	}
	w.Header().Set(ContentTypeHeader, jsonhttp.DefaultContentTypeHeader)
	_, _ = io.Copy(w, bytes.NewBuffer(b))
}
