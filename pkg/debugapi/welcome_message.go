// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"io"
	"io/ioutil"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

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
	const maxMessageSize = 140 + 1 // TODO: import this constant from handshake or libp2p

	data, err := ioutil.ReadAll(io.LimitReader(r.Body, maxMessageSize))
	if err != nil {
		s.Logger.Debugf("debugapi: welcome message: error reading request: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}
	if len(data) > 140 { // TODO: import this constant from handshake or libp2p
		s.Logger.Debugf("debugapi: welcome message: welcome message length exceeds maximum of 140")
		jsonhttp.BadRequest(w, "welcome message length exceeds maximum of 140")
		return
	}

	if err := s.P2P.SetWelcomeMessage(string(data)); err != nil {
		s.Logger.Debugf("debugapi: welcome message: failed to set: %v", err)
		s.Logger.Errorf("Failed to set welcome message")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
