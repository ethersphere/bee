// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"encoding/json"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

const welcomeMessageMaxRequestSize = 512

type welcomeMessageRequest struct {
	WelcomeMesssage string `json:"welcome_message"`
}

type welcomeMessageResponse struct {
	WelcomeMesssage string `json:"welcome_message"`
}

func (s *server) getWelcomeMessageHandler(w http.ResponseWriter, r *http.Request) {
	val := s.P2P.GetWelcomeMessage()
	jsonhttp.OK(w, welcomeMessageResponse{
		WelcomeMesssage: val,
	})
}

func (s *server) setWelcomeMessageHandler(w http.ResponseWriter, r *http.Request) {
	var data welcomeMessageRequest
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		s.Logger.Debugf("debugapi: welcome message: failed to read request: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if err := s.P2P.SetWelcomeMessage(data.WelcomeMesssage); err != nil {
		s.Logger.Debugf("debugapi: welcome message: failed to set: %v", err)
		s.Logger.Errorf("Failed to set welcome message")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
