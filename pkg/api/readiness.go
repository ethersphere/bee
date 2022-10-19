// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import "net/http"

func (s *Service) readinessHandler(w http.ResponseWriter, _ *http.Request) {
	if s.probe.Ready() == ProbeStatusOK {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
}
