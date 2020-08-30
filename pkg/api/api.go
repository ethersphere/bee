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
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
)

// Swarm headers.
const (
	SwarmPinHeader     = "Swarm-Pin"
	SwarmTagUIDHeader  = "Swarm-Tag-Uid"
	SwarmEncryptHeader = "Swarm-Encrypt"
)

// Service is the API service interface.
type Service interface {
	http.Handler
	m.Collector
}

type server struct {
	Tags               *tags.Tags
	Storer             storage.Storer
	Resolver           resolver.Interface
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

// New will create a and initialize a new API service.
func New(tags *tags.Tags, storer storage.Storer, resolver resolver.Interface, corsAllowedOrigins []string, logger logging.Logger, tracer *tracing.Tracer) Service {
	s := &server{
		Tags:               tags,
		Storer:             storer,
		Resolver:           resolver,
		CORSAllowedOrigins: corsAllowedOrigins,
		Logger:             logger,
		Tracer:             tracer,
		metrics:            newMetrics(),
	}

	s.setupRouting()

	return s
}

// getOrCreateTag attempts to get the tag if an id is supplied, and returns an error if it does not exist.
// If no id is supplied, it will attempt to create a new tag with a generated name and return it.
func (s *server) getOrCreateTag(tagUID string) (*tags.Tag, bool, error) {
	// if tag ID is not supplied, create a new tag
	if tagUID == "" {
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		var err error
		tag, err := s.Tags.Create(tagName, 0)
		if err != nil {
			return nil, false, fmt.Errorf("cannot create tag: %w", err)
		}
		return tag, true, nil
	}
	uid, err := strconv.Atoi(tagUID)
	if err != nil {
		return nil, false, fmt.Errorf("cannot parse taguid: %w", err)
	}
	t, err := s.Tags.Get(uint32(uid))
	return t, false, err
}

var errInvalidChunkAddress = errors.New("invalid chunk address")
var errNoResolver = errors.New("no resolver connected")

func (s *server) resolveNameOrAddress(str string) (swarm.Address, error) {
	log := s.Logger

	// Try and parse the name as a bzz address.
	adr, err := swarm.ParseHexAddress(str)
	if err == nil {
		log.Debugf("name resolve: valid bzz address %q", str)
		return adr, nil
	}

	// If no resolver is not available, return an error.
	if s.Resolver == nil {
		log.Errorf("name resolve: no name resolver available", str)
		return swarm.ZeroAddress, errNoResolver
	}

	// Try and resolve the name using the provided resolver.
	log.Debugf("name resolve: attempting to resolve %s to bzz address", str)
	adr, err = s.Resolver.Resolve(str)
	if err == nil && !adr.IsZero() {
		log.Infof("name resolve: resolved name %s to %s", str, adr)
		return adr, nil
	}
	log.Errorf("name resolve: failed to resolve name %s", str)

	return swarm.ZeroAddress, fmt.Errorf("%w: %v", errInvalidChunkAddress, err)
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
