// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/logging"
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/traversal"
)

const (
	SwarmPinHeader            = "Swarm-Pin"
	SwarmTagUidHeader         = "Swarm-Tag-Uid"
	SwarmEncryptHeader        = "Swarm-Encrypt"
	SwarmIndexDocumentHeader  = "Swarm-Index-Document"
	SwarmErrorDocumentHeader  = "Swarm-Error-Document"
	SwarmPostageBatchIdHeader = "Swarm-Postage-Batch-Id"
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
	errInvalidPostageBatch  = errors.New("invalid postage batch id")
)

// Service is the API service interface.
type Service interface {
	http.Handler
	m.Collector
	io.Closer
}

type server struct {
	Tags      *tags.Tags
	Storer    storage.Storer
	Resolver  resolver.Interface
	Pss       pss.Interface
	Traversal traversal.Service
	Logger    logging.Logger
	Tracer    *tracing.Tracer
	signer    crypto.Signer
	post      postage.Service
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
func New(tags *tags.Tags, storer storage.Storer, resolver resolver.Interface, pss pss.Interface, traversalService traversal.Service, post postage.Service, signer crypto.Signer, logger logging.Logger, tracer *tracing.Tracer, o Options) Service {
	s := &server{
		Tags:      tags,
		Storer:    storer,
		Resolver:  resolver,
		Pss:       pss,
		Traversal: traversalService,
		post:      post,
		signer:    signer,
		Options:   o,
		Logger:    logger,
		Tracer:    tracer,
		metrics:   newMetrics(),
		quit:      make(chan struct{}),
	}

	s.setupRouting()

	return s
}

// Close hangs up running websockets on shutdown.
func (s *server) Close() error {
	s.Logger.Info("api shutting down")
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
		tagName := fmt.Sprintf("unnamed_tag_%d", time.Now().Unix())
		var err error
		tag, err := s.Tags.Create(tagName, 0)
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
	return s.Tags.Get(uint32(uid))
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

func requestPostageBatchId(r *http.Request) ([]byte, error) {
	if h := strings.ToLower(r.Header.Get(SwarmPostageBatchIdHeader)); h != "" {
		if len(h) != 64 {
			return nil, errInvalidPostageBatch
		}
		b, err := hex.DecodeString(h)
		if err != nil {
			return nil, errInvalidPostageBatch
		}
		return b, nil
	}

	// fallback to a slice of 32 zeros
	return make([]byte, 32), nil
}

func (s *server) newTracingHandler(spanName string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, err := s.Tracer.WithContextFromHTTPHeaders(r.Context(), r.Header)
			if err != nil && !errors.Is(err, tracing.ErrContextNotFound) {
				s.Logger.Debugf("span '%s': extract tracing context: %v", spanName, err)
				// ignore
			}

			span, _, ctx := s.Tracer.StartSpanFromContext(ctx, spanName, s.Logger)
			defer span.Finish()

			err = s.Tracer.AddContextHTTPHeader(ctx, r.Header)
			if err != nil {
				s.Logger.Debugf("span '%s': inject tracing context: %v", spanName, err)
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

type stamperPutter struct {
	storage.Storer
	stamper postage.Stamper
}

func newStamperPutter(s storage.Storer, post postage.Service, signer crypto.Signer, batch []byte) (storage.Storer, error) {
	i, err := post.GetStampIssuer(batch)
	if err != nil {
		return nil, fmt.Errorf("stamp issuer: %w", err)
	}

	stamper := postage.NewStamper(i, signer)
	return &stamperPutter{Storer: s, stamper: stamper}, nil
}

func (p *stamperPutter) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for i, c := range chs {
		stamp, err := p.stamper.Stamp(c.Address())
		if err != nil {
			return nil, err
		}
		chs[i] = c.WithStamp(stamp)
	}

	return p.Storer.Put(ctx, mode, chs...)
}

type pipelineFunc func(context.Context, io.Reader, int64) (swarm.Address, error)

func requestPipelineFn(s storage.Storer, r *http.Request) pipelineFunc {
	mode, encrypt := requestModePut(r), requestEncrypt(r)
	return func(ctx context.Context, r io.Reader, l int64) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, mode, encrypt)
		return builder.FeedPipeline(ctx, pipe, r, l)
	}
}
