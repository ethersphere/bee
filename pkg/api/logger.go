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

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/log"
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

	logger struct {
		Logger    string `json:"logger"`
		Verbosity string `json:"verbosity"`
		Subsystem string `json:"subsystem"`
		ID        string `json:"id"`
	}

	loggerResult struct {
		Tree    node     `json:"tree"`
		Loggers []logger `json:"loggers"`
	}
)

// loggerGetHandler returns all available loggers that match the specified expression.
func (s *Service) loggerGetHandler(w http.ResponseWriter, r *http.Request) {
	var (
		exp string
		err error
	)

	if val, ok := mux.Vars(r)["exp"]; ok {
		buf, err := base64.URLEncoding.DecodeString(val)
		if err != nil {
			s.logger.Debugf("loggers get: query parameter decoding failed: %v", err)
			s.logger.Error("loggers get: query parameter decoding failed")
			jsonhttp.BadRequest(w, err)
		}
		exp = string(buf)
	}

	rex, err := regexp.Compile(exp)

	result := loggerResult{Tree: make(node)}
	logRegistryIterate(func(id, name string, verbosity log.Level, v uint) bool {
		if exp == "" || exp == id || (rex != nil && rex.MatchString(id)) {
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
			result.Loggers = append(result.Loggers, logger{
				Logger:    name,
				Verbosity: verbosity.String(),
				Subsystem: id,
				ID:        base64.URLEncoding.EncodeToString([]byte(id)),
			})
		}
		return true
	})

	if len(result.Loggers) == 0 && err != nil {
		s.logger.Debugf("loggers get: regexp compilation failed: %v", err)
		s.logger.Error("loggers get: regexp compilation failed")
		jsonhttp.BadRequest(w, err)
	} else {
		jsonhttp.OK(w, result)
	}
}

// loggerSetVerbosityHandler sets logger(s) verbosity level based on
// the specified expression or subsystem that matches the logger(s).
func (s *Service) loggerSetVerbosityHandler(w http.ResponseWriter, r *http.Request) {
	verbosity, err := log.ParseVerbosityLevel(mux.Vars(r)["verbosity"])
	if err != nil {
		s.logger.Debugf("loggers set verbosity: parse verbosity level failed: %v", err)
		s.logger.Error("loggers set verbosity: parse verbosity level failed")
		jsonhttp.BadRequest(w, err)
	}

	exp, err := base64.URLEncoding.DecodeString(mux.Vars(r)["exp"])
	if err != nil {
		s.logger.Debugf("loggers set verbosity: query parameter decoding failed: %v", err)
		s.logger.Error("loggers set verbosity: query parameter decoding failed")
		jsonhttp.BadRequest(w, err)
	}

	if err := logSetVerbosityByExp(string(exp), verbosity); err != nil {
		jsonhttp.BadRequest(w, err)
	} else {
		jsonhttp.OK(w, nil)
	}
}
