// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/prometheus/client_golang/prometheus"
	"resenje.org/web"
)

type Service interface {
	http.Handler
	Metrics() (cs []prometheus.Collector)
}

type server struct {
	Options
	http.Handler
	metrics metrics
}

type Options struct {
	Pingpong pingpong.Interface
	Logger   interface {
		Debugf(format string, args ...interface{})
		Errorf(format string, args ...interface{})
	}
}

func New(o Options) Service {
	s := &server{
		Options: o,
		metrics: newMetrics(),
	}

	s.setupRouting()

	return s
}

type methodHandler map[string]http.Handler

func (h methodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	web.HandleMethods(h, `{"message":"Method Not Allowed","code":405}`, jsonhttp.DefaultContentTypeHeader, w, r)
}
