// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strings"

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
	Tags               *tags.Tags
	Storer             storage.Storer
	CORSAllowedOrigins []string
	Logger             logging.Logger
	Tracer             *tracing.Tracer
	http.Handler
	metrics metrics
}

const (
	// TargetsRecoveryHeader defines the Header for Recovery targets in Global Pinning
	TargetsRecoveryHeader = "swarm-recovery-targets"
)

func New(tags *tags.Tags, storer storage.Storer, corsAllowedOrigins []string, logger logging.Logger, tracer *tracing.Tracer) Service {
	s := &server{
		Tags:               tags,
		Storer:             storer,
		CORSAllowedOrigins: corsAllowedOrigins,
		Logger:             logger,
		Tracer:             tracer,
		metrics:            newMetrics(),
	}

	s.setupRouting()

	return s
}

const (
	SwarmPinHeader = "Swarm-Pin"
	TagHeaderUid   = "swarm-tag-uid"
)

// requestModePut returns the desired storage.ModePut for this request based on the request headers.
func requestModePut(r *http.Request) storage.ModePut {
	if h := strings.ToLower(r.Header.Get(SwarmPinHeader)); h == "true" {
		return storage.ModePutUploadPin
	}
	return storage.ModePutUpload
}
