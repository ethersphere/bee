// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

func (s *Service) reserveStateHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, s.batchStore.GetReserveState())
}
