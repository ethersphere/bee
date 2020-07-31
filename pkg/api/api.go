// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
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
	Tags               *tags.Tags
	Storer             storage.Storer
	CORSAllowedOrigins []string
	Logger             logging.Logger
	Tracer             *tracing.Tracer
}

const (
	// TargetsRecoveryHeader defines the Header for Recovery targets in Global Pinning
	TargetsRecoveryHeader = "swarm-recovery-targets"
)

func New(o Options) Service {
	s := &server{
		Options: o,
		metrics: newMetrics(),
	}

	s.setupRouting()

	return s
}
