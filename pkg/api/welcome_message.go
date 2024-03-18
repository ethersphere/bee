// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

const welcomeMessageMaxRequestSize = 512

type welcomeMessageRequest struct {
	WelcomeMesssage string `json:"welcomeMessage"`
}

type welcomeMessageResponse struct {
	WelcomeMesssage string `json:"welcomeMessage"`
}

func (s *Service) getWelcomeMessageHandler(w http.ResponseWriter, _ *http.Request) {
	val := s.p2p.GetWelcomeMessage()
	jsonhttp.OK(w, welcomeMessageResponse{
		WelcomeMesssage: val,
	})
}

func (s *Service) setWelcomeMessageHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_welcome_message").Build()

	var data welcomeMessageRequest
	err := json.NewDecoder(r.Body).Decode(&data)
	if err != nil {
		logger.Debug("failed to read body", "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if err := s.p2p.SetWelcomeMessage(data.WelcomeMesssage); err != nil {
		logger.Debug("set welcome message failed", "error", err)
		logger.Error(nil, "set welcome message failed")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
