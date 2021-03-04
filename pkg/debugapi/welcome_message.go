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

func (s *Service) getWelcomeMessageHandler(w http.ResponseWriter, r *http.Request) {
	val := s.p2p.GetWelcomeMessage()
	jsonhttp.OK(w, welcomeMessageResponse{
		WelcomeMesssage: val,
	})
}

func (s *Service) setWelcomeMessageHandler(w http.ResponseWriter, r *http.Request) {
	var data welcomeMessageRequest
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		s.logger.Debugf("debugapi: welcome message: failed to read request: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if err := s.p2p.SetWelcomeMessage(data.WelcomeMesssage); err != nil {
		s.logger.Debugf("debugapi: welcome message: failed to set: %v", err)
		s.logger.Errorf("Failed to set welcome message")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
