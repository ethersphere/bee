// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/websocket"
)

const (
	SwarmPinHeader     = "Swarm-Pin"
	SwarmTagUidHeader  = "Swarm-Tag-Uid"
	SwarmEncryptHeader = "Swarm-Encrypt"
	SwarmIndextHeader  = "Swarm-Index"
)

var (
	errInvalidNameOrAddress = errors.New("invalid name or bzz address")
	errNoResolver           = errors.New("no resolver connected")
)

// Service is the API service interface.
type Service interface {
	http.Handler
	m.Collector
}

type server struct {
	Tags     *tags.Tags
	Storer   storage.Storer
	Resolver resolver.Interface
	Pss      pss.Interface
	Logger   logging.Logger
	Tracer   *tracing.Tracer
	Options
	http.Handler
	metrics metrics

	wsWg      sync.WaitGroup // wait for all websockets to close on exit
	wsHandles map[string]*websocket.Conn
}

type Options struct {
	CORSAllowedOrigins []string
	GatewayMode        bool
}

const (
	// TargetsRecoveryHeader defines the Header for Recovery targets in Global Pinning
	TargetsRecoveryHeader = "swarm-recovery-targets"
)

// New will create a and initialize a new API service.
func New(tags *tags.Tags, storer storage.Storer, resolver resolver.Interface, pss pss.Interface, logger logging.Logger, tracer *tracing.Tracer, o Options) Service {
	s := &server{
		Tags:     tags,
		Storer:   storer,
		Resolver: resolver,
		Pss:      pss,
		Options:  o,
		Logger:   logger,
		Tracer:   tracer,
		metrics:  newMetrics(),
	}

	s.setupRouting()

	return s
}

// getOrCreateTag attempts to get the tag if an id is supplied, and returns an error if it does not exist.
// If no id is supplied, it will attempt to create a new tag with a generated name and return it.
func (s *server) getOrCreateTag(tagUid string) (*tags.Tag, bool, error) {
	// if tag ID is not supplied, create a new tag
	if tagUid == "" {
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		var err error
		tag, err := s.Tags.Create(tagName, 0)
		if err != nil {
			return nil, false, fmt.Errorf("cannot create tag: %w", err)
		}
		return tag, true, nil
	}
	uid, err := strconv.Atoi(tagUid)
	if err != nil {
		return nil, false, fmt.Errorf("cannot parse taguid: %w", err)
	}
	t, err := s.Tags.Get(uint32(uid))
	return t, false, err
}

func (s *server) resolveNameOrAddress(str string) (swarm.Address, error) {
	log := s.Logger

	// Try and parse the name as a bzz address.
	addr, err := swarm.ParseHexAddress(str)
	if err == nil {
		log.Tracef("name resolve: valid bzz address %q", str)
		return addr, nil
	}

	// If no resolver is not available, return an error.
	if s.Resolver == nil {
		return swarm.ZeroAddress, errNoResolver
	}

	// Try and resolve the name using the provided resolver.
	log.Debugf("name resolve: attempting to resolve %s to bzz address", str)
	addr, err = s.Resolver.Resolve(str)
	if err == nil {
		log.Tracef("name resolve: resolved name %s to %s", str, addr)
		return addr, nil
	}

	return swarm.ZeroAddress, fmt.Errorf("%w: %v", errInvalidNameOrAddress, err)
}

// requestModePut returns the desired storage.ModePut for this request based on the request headers.
func requestModePut(r *http.Request) storage.ModePut {
	if h := strings.ToLower(r.Header.Get(SwarmPinHeader)); h == "true" {
		return storage.ModePutUploadPin
	}
	return storage.ModePutUpload
}

func requestEncrypt(r *http.Request) bool {
	return strings.ToLower(r.Header.Get(SwarmEncryptHeader)) == "true"
}

func (s *server) newTracingHandler(spanName string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span, _, ctx := s.Tracer.StartSpanFromContext(r.Context(), spanName, s.Logger)
			defer span.Finish()

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
