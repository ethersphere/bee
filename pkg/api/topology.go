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
	"github.com/ethersphere/bee/v2/pkg/topology"
)

func (s *Service) topologyHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_topology").Build()

	params, err := s.topology()
	if err != nil {
		logger.Error(err, "marshal to json failed")
		jsonhttp.InternalServerError(w, err)
		return
	}

	b, err := json.Marshal(params)
	if err != nil {
		logger.Error(err, "marshal to json failed")
		jsonhttp.InternalServerError(w, err)
		return
	}
	w.Header().Set(ContentTypeHeader, jsonhttp.DefaultContentTypeHeader)
	_, _ = io.Copy(w, bytes.NewBuffer(b))
}

func (s *Service) topology() (*topology.KadParams, error) {
	params := s.topologyDriver.Snapshot()
	params.LightNodes = s.lightNodes.PeerInfo()
	return params, nil
}
