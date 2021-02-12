// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api provides the functionality of the Bee
// client-facing HTTP API.
package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/traversal"
)

const (
	SwarmPinHeader           = "Swarm-Pin"
	SwarmTagHeader           = "Swarm-Tag"
	SwarmEncryptHeader       = "Swarm-Encrypt"
	SwarmIndexDocumentHeader = "Swarm-Index-Document"
	SwarmErrorDocumentHeader = "Swarm-Error-Document"
	SwarmFeedIndexHeader     = "Swarm-Feed-Index"
	SwarmFeedIndexNextHeader = "Swarm-Feed-Index-Next"
)

// The size of buffer used for prefetching content with Langos.
// Warning: This value influences the number of chunk requests and chunker join goroutines
// per file request.
// Recommended value is 8 or 16 times the io.Copy default buffer value which is 32kB, depending
// on the file size. Use lookaheadBufferSize() to get the correct buffer size for the request.
const (
	smallFileBufferSize = 8 * 32 * 1024
	largeFileBufferSize = 16 * 32 * 1024

	largeBufferFilesizeThreshold = 10 * 1000000 // ten megs
)

var (
	errInvalidNameOrAddress = errors.New("invalid name or bzz address")
	errNoResolver           = errors.New("no resolver connected")
)

// Service is the API service interface.
type Service interface {
	http.Handler
	m.Collector
	io.Closer
}

type server struct {
	tags        *tags.Tags
	storer      storage.Storer
	resolver    resolver.Interface
	pss         pss.Interface
	traversal   traversal.Service
	logger      logging.Logger
	tracer      *tracing.Tracer
	feedFactory feeds.Factory
	Options
	http.Handler
	metrics metrics

	wsWg sync.WaitGroup // wait for all websockets to close on exit
	quit chan struct{}
}

type Options struct {
	CORSAllowedOrigins []string
	GatewayMode        bool
	WsPingPeriod       time.Duration
}

const (
	// TargetsRecoveryHeader defines the Header for Recovery targets in Global Pinning
	TargetsRecoveryHeader = "swarm-recovery-targets"
)

// New will create a and initialize a new API service.
func New(tags *tags.Tags, storer storage.Storer, resolver resolver.Interface, pss pss.Interface, traversalService traversal.Service, feedFactory feeds.Factory, logger logging.Logger, tracer *tracing.Tracer, o Options) Service {
	s := &server{
		tags:        tags,
		storer:      storer,
		resolver:    resolver,
		pss:         pss,
		traversal:   traversalService,
		feedFactory: feedFactory,
		Options:     o,
		logger:      logger,
		tracer:      tracer,
		metrics:     newMetrics(),
		quit:        make(chan struct{}),
	}

	s.setupRouting()

	return s
}

// Close hangs up running websockets on shutdown.
func (s *server) Close() error {
	s.logger.Info("api shutting down")
	close(s.quit)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wsWg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return errors.New("api shutting down with open websockets")
	}

	return nil
}

// getOrCreateTag attempts to get the tag if an id is supplied, and returns an error if it does not exist.
// If no id is supplied, it will attempt to create a new tag with a generated name and return it.
func (s *server) getOrCreateTag(tagUid string) (*tags.Tag, bool, error) {
	// if tag ID is not supplied, create a new tag
	if tagUid == "" {
		tag, err := s.tags.Create(0)
		if err != nil {
			return nil, false, fmt.Errorf("cannot create tag: %w", err)
		}
		return tag, true, nil
	}
	t, err := s.getTag(tagUid)
	return t, false, err
}

func (s *server) getTag(tagUid string) (*tags.Tag, error) {
	uid, err := strconv.Atoi(tagUid)
	if err != nil {
		return nil, fmt.Errorf("cannot parse taguid: %w", err)
	}
	return s.tags.Get(uint32(uid))
}

func (s *server) resolveNameOrAddress(str string) (swarm.Address, error) {
	log := s.logger

	// Try and parse the name as a bzz address.
	addr, err := swarm.ParseHexAddress(str)
	if err == nil {
		log.Tracef("name resolve: valid bzz address %q", str)
		return addr, nil
	}

	// If no resolver is not available, return an error.
	if s.resolver == nil {
		return swarm.ZeroAddress, errNoResolver
	}

	// Try and resolve the name using the provided resolver.
	log.Debugf("name resolve: attempting to resolve %s to bzz address", str)
	addr, err = s.resolver.Resolve(str)
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
			ctx, err := s.tracer.WithContextFromHTTPHeaders(r.Context(), r.Header)
			if err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
				s.logger.Debugf("span '%s': extract tracing context: %v", spanName, err)
				// ignore
			}

			span, _, ctx := s.tracer.StartSpanFromContext(ctx, spanName, s.logger)
			defer span.Finish()

			err = s.tracer.AddContextHTTPHeader(ctx, r.Header)
			if err != nil {
				s.logger.Debugf("span '%s': inject tracing context: %v", spanName, err)
				// ignore
			}

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func lookaheadBufferSize(size int64) int {
	if size <= largeBufferFilesizeThreshold {
		return smallFileBufferSize
	}
	return largeFileBufferSize
}

// checkOrigin returns true if the origin is not set or is equal to the request host.
func (s *server) checkOrigin(r *http.Request) bool {
	origin := r.Header["Origin"]
	if len(origin) == 0 {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	hosts := append(s.CORSAllowedOrigins, scheme+"://"+r.Host)
	for _, v := range hosts {
		if equalASCIIFold(origin[0], v) || v == "*" {
			return true
		}
	}

	return false
}

// equalASCIIFold returns true if s is equal to t with ASCII case folding as
// defined in RFC 4790.
func equalASCIIFold(s, t string) bool {
	for s != "" && t != "" {
		sr, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		tr, size := utf8.DecodeRuneInString(t)
		t = t[size:]
		if sr == tr {
			continue
		}
		if 'A' <= sr && sr <= 'Z' {
			sr = sr + 'a' - 'A'
		}
		if 'A' <= tr && tr <= 'Z' {
			tr = tr + 'a' - 'A'
		}
		if sr != tr {
			return false
		}
	}
	return s == t
}

type pipelineFunc func(context.Context, io.Reader, int64) (swarm.Address, error)

func requestPipelineFn(s storage.Storer, r *http.Request) pipelineFunc {
	mode, encrypt := requestModePut(r), requestEncrypt(r)
	return func(ctx context.Context, r io.Reader, l int64) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, mode, encrypt)
		return builder.FeedPipeline(ctx, pipe, r, l)
	}
}

// calculateNumberOfChunks calculates the number of chunks in an arbitrary
// content length.
func calculateNumberOfChunks(contentLength int64, isEncrypted bool) int64 {
	if contentLength <= swarm.ChunkSize {
		return 1
	}
	branchingFactor := swarm.Branches
	if isEncrypted {
		branchingFactor = swarm.EncryptedBranches
	}

	dataChunks := math.Ceil(float64(contentLength) / float64(swarm.ChunkSize))
	totalChunks := dataChunks
	intermediate := dataChunks / float64(branchingFactor)

	for intermediate > 1 {
		totalChunks += math.Ceil(intermediate)
		intermediate = intermediate / float64(branchingFactor)
	}

	return int64(totalChunks) + 1
}

func requestCalculateNumberOfChunks(r *http.Request) int64 {
	if !strings.Contains(r.Header.Get(contentTypeHeader), "multipart") && r.ContentLength > 0 {
		return calculateNumberOfChunks(r.ContentLength, requestEncrypt(r))
	}
	return 0
}
