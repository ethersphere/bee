// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
)

type Service interface {
	http.Handler
	m.Collector
}

type server struct {
	Options
	http.Handler
	metrics metrics
}

type Options struct {
	Pingpong pingpong.Interface
	Tags     *tags.Tags
	Storer   storage.Storer
	Logger   logging.Logger
	Tracer   *tracing.Tracer
}

func New(o Options) Service {
	s := &server{
		Options: o,
		metrics: newMetrics(),
	}

	s.setupRouting()

	return s
}
