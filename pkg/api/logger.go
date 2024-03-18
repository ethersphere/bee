// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/gorilla/mux"
)

// The following variables exist only to be mocked in tests.
var (
	logRegistryIterate   = log.RegistryIterate
	logSetVerbosityByExp = log.SetVerbosityByExp
)

type (
	data struct {
		Next  node     `json:"/,omitempty"`
		Names []string `json:"+,omitempty"`
	}

	node map[string]*data

	loggerInfo struct {
		Logger    string `json:"logger"`
		Verbosity string `json:"verbosity"`
		Subsystem string `json:"subsystem"`
		ID        string `json:"id"`
	}

	loggerResult struct {
		Tree    node         `json:"tree"`
		Loggers []loggerInfo `json:"loggers"`
	}
)

// loggerGetHandler returns all available loggers that match the specified expression.
func (s *Service) loggerGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_loggers").Build()

	paths := struct {
		Exp string `map:"exp,decBase64url"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	rex, err := regexp.Compile(paths.Exp)

	result := loggerResult{Tree: make(node)}
	logRegistryIterate(func(id, name string, verbosity log.Level, v uint) bool {
		if paths.Exp == id || (rex != nil && rex.MatchString(id)) {
			if int(verbosity) == int(v) {
				verbosity = log.VerbosityAll
			}

			// Tree structure.
			curr := result.Tree
			path := strings.Split(name, "/")
			last := len(path) - 1
			for i, n := range path {
				if curr[n] == nil {
					curr[n] = &data{Next: make(node)}
				}
				if i == last {
					name := fmt.Sprintf("%s|%s", verbosity, id)
					curr[n].Names = append(curr[n].Names, name)
				}
				curr = curr[n].Next
			}

			// Flat structure.
			result.Loggers = append(result.Loggers, loggerInfo{
				Logger:    name,
				Verbosity: verbosity.String(),
				Subsystem: id,
				ID:        base64.URLEncoding.EncodeToString([]byte(id)),
			})
		}
		return true
	})

	if len(result.Loggers) == 0 && err != nil {
		logger.Debug("invalid path params", "error", err)
		logger.Error(nil, "invalid path params")
		jsonhttp.BadRequest(w, jsonhttp.StatusResponse{
			Message: "invalid path params",
			Code:    http.StatusBadRequest,
			Reasons: []jsonhttp.Reason{{
				Field: "exp",
				Error: err.Error(),
			}},
		})
	} else {
		jsonhttp.OK(w, result)
	}
}

// loggerSetVerbosityHandler sets logger(s) verbosity level based on
// the specified expression or subsystem that matches the logger(s).
func (s *Service) loggerSetVerbosityHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("put_loggers").Build()

	paths := struct {
		Exp       string `map:"exp,decBase64url" validate:"required"`
		Verbosity string `map:"verbosity" validate:"required,oneof=none error warning info debug all"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	if err := logSetVerbosityByExp(paths.Exp, log.MustParseVerbosityLevel(paths.Verbosity)); err != nil {
		logger.Debug("invalid path params", "error", err)
		logger.Error(nil, "invalid path params")
		jsonhttp.BadRequest(w, jsonhttp.StatusResponse{
			Message: "invalid path params",
			Code:    http.StatusBadRequest,
			Reasons: []jsonhttp.Reason{{
				Field: "exp",
				Error: err.Error(),
			}},
		})
	} else {
		jsonhttp.OK(w, nil)
	}
}
