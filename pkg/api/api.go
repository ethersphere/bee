// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api provides the functionality of the Bee
// client-facing HTTP API.
package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/accounting"
	"github.com/ethersphere/bee/pkg/auth"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/pingpong"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pusher"
	"github.com/ethersphere/bee/pkg/resolver"
	"github.com/ethersphere/bee/pkg/settlement"
	"github.com/ethersphere/bee/pkg/settlement/swap"
	"github.com/ethersphere/bee/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/pkg/steward"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/topology/lightnode"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/ethersphere/bee/pkg/transaction"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const (
	SwarmPinHeader            = "Swarm-Pin"
	SwarmTagHeader            = "Swarm-Tag"
	SwarmEncryptHeader        = "Swarm-Encrypt"
	SwarmIndexDocumentHeader  = "Swarm-Index-Document"
	SwarmErrorDocumentHeader  = "Swarm-Error-Document"
	SwarmFeedIndexHeader      = "Swarm-Feed-Index"
	SwarmFeedIndexNextHeader  = "Swarm-Feed-Index-Next"
	SwarmCollectionHeader     = "Swarm-Collection"
	SwarmPostageBatchIdHeader = "Swarm-Postage-Batch-Id"
	SwarmDeferredUploadHeader = "Swarm-Deferred-Upload"
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

	uploadSem = 50
)

const (
	contentTypeHeader = "Content-Type"
	multiPartFormData = "multipart/form-data"
	contentTypeTar    = "application/x-tar"
)

var (
	errInvalidNameOrAddress = errors.New("invalid name or bzz address")
	errNoResolver           = errors.New("no resolver connected")
	errInvalidRequest       = errors.New("could not validate request")
	errInvalidContentType   = errors.New("invalid content-type")
	errDirectoryStore       = errors.New("could not store directory")
	errFileStore            = errors.New("could not store file")
	errInvalidPostageBatch  = errors.New("invalid postage batch id")
	errBatchUnusable        = errors.New("batch not usable")
)

type authenticator interface {
	Authorize(string) bool
	GenerateKey(string, int) (string, error)
	RefreshKey(string, int) (string, error)
	Enforce(string, string, string) (bool, error)
}

type Service struct {
	auth            authenticator
	tags            *tags.Tags
	storer          storage.Storer
	resolver        resolver.Interface
	pss             pss.Interface
	traversal       traversal.Traverser
	pinning         pinning.Interface
	steward         steward.Interface
	logger          logging.Logger
	tracer          *tracing.Tracer
	feedFactory     feeds.Factory
	signer          crypto.Signer
	post            postage.Service
	postageContract postagecontract.Interface
	chunkPushC      chan *pusher.Op
	metricsRegistry *prometheus.Registry
	Options

	http.Handler
	router *mux.Router

	metrics metrics

	wsWg sync.WaitGroup // wait for all websockets to close on exit
	quit chan struct{}

	// from debug API
	overlay           *swarm.Address
	publicKey         ecdsa.PublicKey
	pssPublicKey      ecdsa.PublicKey
	ethereumAddress   common.Address
	chequebookEnabled bool
	swapEnabled       bool

	topologyDriver topology.Driver
	p2p            p2p.DebugService
	accounting     accounting.Interface
	chequebook     chequebook.Service
	pseudosettle   settlement.Interface
	pingpong       pingpong.Interface

	batchStore        postage.Storer
	batchEventUpdater postage.SyncStatus

	swap        swap.Interface
	transaction transaction.Service
	lightNodes  *lightnode.Container
	blockTime   *big.Int

	postageSem       *semaphore.Weighted
	cashOutChequeSem *semaphore.Weighted
	beeMode          BeeNodeMode
	gatewayMode      bool

	chainBackend transaction.Backend
	erc20Service erc20.Service
	chainID      int64
}

func (s *Service) SetP2P(p2p p2p.DebugService) {
	if s != nil {
		s.p2p = p2p
	}
}

func (s *Service) SetSwarmAddress(addr *swarm.Address) {
	if s != nil {
		s.overlay = addr
	}
}

type Options struct {
	CORSAllowedOrigins []string
	GatewayMode        bool
	WsPingPeriod       time.Duration
	Restricted         bool
}

type ExtraOptions struct {
	Pingpong          pingpong.Interface
	TopologyDriver    topology.Driver
	LightNodes        *lightnode.Container
	Accounting        accounting.Interface
	Pseudosettle      settlement.Interface
	Swap              swap.Interface
	Chequebook        chequebook.Service
	BlockTime         *big.Int
	Tags              *tags.Tags
	Storer            storage.Storer
	Resolver          resolver.Interface
	Pss               pss.Interface
	TraversalService  traversal.Traverser
	Pinning           pinning.Interface
	FeedFactory       feeds.Factory
	Post              postage.Service
	PostageContract   postagecontract.Interface
	Steward           steward.Interface
	BatchEventUpdater postage.SyncStatus
}

func New(publicKey, pssPublicKey ecdsa.PublicKey, ethereumAddress common.Address, logger logging.Logger, transaction transaction.Service, batchStore postage.Storer, gatewayMode bool, beeMode BeeNodeMode, chequebookEnabled bool, swapEnabled bool, cors []string) *Service {
	s := new(Service)

	s.CORSAllowedOrigins = cors
	s.beeMode = beeMode
	s.gatewayMode = gatewayMode
	s.logger = logger
	s.chequebookEnabled = chequebookEnabled
	s.swapEnabled = swapEnabled
	s.publicKey = publicKey
	s.pssPublicKey = pssPublicKey
	s.ethereumAddress = ethereumAddress
	s.transaction = transaction
	s.batchStore = batchStore
	s.metricsRegistry = newDebugMetrics()

	return s
}

// Configure will create a and initialize a new API service.
func (s *Service) Configure(signer crypto.Signer, auth authenticator, tracer *tracing.Tracer, o Options, e ExtraOptions, chainID int64, chainBackend transaction.Backend, erc20 erc20.Service) <-chan *pusher.Op {
	s.auth = auth
	s.chunkPushC = make(chan *pusher.Op)
	s.signer = signer
	s.Options = o
	s.tracer = tracer
	s.metrics = newMetrics()

	s.quit = make(chan struct{})

	s.tags = e.Tags
	s.storer = e.Storer
	s.resolver = e.Resolver
	s.pss = e.Pss
	s.traversal = e.TraversalService
	s.pinning = e.Pinning
	s.feedFactory = e.FeedFactory
	s.post = e.Post
	s.postageContract = e.PostageContract
	s.steward = e.Steward

	s.pingpong = e.Pingpong
	s.topologyDriver = e.TopologyDriver
	s.accounting = e.Accounting
	s.chequebook = e.Chequebook
	s.swap = e.Swap
	s.lightNodes = e.LightNodes
	s.pseudosettle = e.Pseudosettle
	s.blockTime = e.BlockTime

	s.postageSem = semaphore.NewWeighted(1)
	s.cashOutChequeSem = semaphore.NewWeighted(1)

	s.chainID = chainID
	s.erc20Service = erc20
	s.chainBackend = chainBackend
	s.batchEventUpdater = e.BatchEventUpdater

	return s.chunkPushC
}

// Close hangs up running websockets on shutdown.
func (s *Service) Close() error {
	s.logger.Info("api shutting down")
	close(s.quit)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wsWg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		return errors.New("api shutting down with open websockets")
	}

	return nil
}

// getOrCreateTag attempts to get the tag if an id is supplied, and returns an error if it does not exist.
// If no id is supplied, it will attempt to create a new tag with a generated name and return it.
func (s *Service) getOrCreateTag(tagUid string) (*tags.Tag, bool, error) {
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

func (s *Service) getTag(tagUid string) (*tags.Tag, error) {
	uid, err := strconv.Atoi(tagUid)
	if err != nil {
		return nil, fmt.Errorf("cannot parse taguid: %w", err)
	}
	return s.tags.Get(uint32(uid))
}

func (s *Service) resolveNameOrAddress(str string) (swarm.Address, error) {
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

func requestDeferred(r *http.Request) (bool, error) {
	if h := strings.ToLower(r.Header.Get(SwarmDeferredUploadHeader)); h != "" {
		return strconv.ParseBool(h)
	}
	return true, nil
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

	return nil, errInvalidPostageBatch
}

type securityTokenRsp struct {
	Key string `json:"key"`
}

type securityTokenReq struct {
	Role   string `json:"role"`
	Expiry int    `json:"expiry"`
}

func (s *Service) authHandler(w http.ResponseWriter, r *http.Request) {
	_, pass, ok := r.BasicAuth()

	if !ok {
		s.logger.Error("api: auth handler: missing basic auth")
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		jsonhttp.Unauthorized(w, "Unauthorized")
		return
	}

	if !s.auth.Authorize(pass) {
		s.logger.Error("api: auth handler: unauthorized")
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		jsonhttp.Unauthorized(w, "Unauthorized")
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.logger.Debugf("api: auth handler: read request body: %v", err)
		s.logger.Error("api: auth handler: read request body")
		jsonhttp.BadRequest(w, "Read request body")
		return
	}

	var payload securityTokenReq
	if err = json.Unmarshal(body, &payload); err != nil {
		s.logger.Debugf("api: auth handler: unmarshal request body: %v", err)
		s.logger.Error("api: auth handler: unmarshal request body")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	key, err := s.auth.GenerateKey(payload.Role, payload.Expiry)
	if errors.Is(err, auth.ErrExpiry) {
		s.logger.Debugf("api: auth handler: generate key: %v", err)
		s.logger.Error("api: auth handler: generate key")
		jsonhttp.BadRequest(w, "Expiry duration must be a positive number")
		return
	}
	if err != nil {
		s.logger.Debugf("api: auth handler: add auth token: %v", err)
		s.logger.Error("api: auth handler: add auth token")
		jsonhttp.InternalServerError(w, "Error generating authorization token")
		return
	}

	jsonhttp.Created(w, securityTokenRsp{
		Key: key,
	})
}

func (s *Service) refreshHandler(w http.ResponseWriter, r *http.Request) {
	reqToken := r.Header.Get("Authorization")
	if !strings.HasPrefix(reqToken, "Bearer ") {
		jsonhttp.Forbidden(w, "Missing bearer token")
		return
	}

	keys := strings.Split(reqToken, "Bearer ")

	if len(keys) != 2 || strings.Trim(keys[1], " ") == "" {
		jsonhttp.Forbidden(w, "Missing security token")
		return
	}

	authToken := keys[1]

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.logger.Debugf("api: auth handler: read request body: %v", err)
		s.logger.Error("api: auth handler: read request body")
		jsonhttp.BadRequest(w, "Read request body")
		return
	}

	var payload securityTokenReq
	if err = json.Unmarshal(body, &payload); err != nil {
		s.logger.Debugf("api: auth handler: unmarshal request body: %v", err)
		s.logger.Error("api: auth handler: unmarshal request body")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	key, err := s.auth.RefreshKey(authToken, payload.Expiry)
	if errors.Is(err, auth.ErrTokenExpired) {
		s.logger.Debugf("api: auth handler: refresh key: %v", err)
		s.logger.Error("api: auth handler: refresh key")
		jsonhttp.BadRequest(w, "Token expired")
		return
	}

	if err != nil {
		s.logger.Debugf("api: auth handler: refresh token: %v", err)
		s.logger.Error("api: auth handler: refresh token")
		jsonhttp.InternalServerError(w, "Error refreshing authorization token")
		return
	}

	jsonhttp.Created(w, securityTokenRsp{
		Key: key,
	})
}

func (s *Service) newTracingHandler(spanName string) func(h http.Handler) http.Handler {
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

func (s *Service) contentLengthMetricMiddleware() func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			h.ServeHTTP(w, r)
			switch r.Method {
			case http.MethodGet:
				hdr := w.Header().Get("Decompressed-Content-Length")
				if hdr == "" {
					s.logger.Debug("api: decompressed content length header not found")
					hdr = w.Header().Get("Content-Length")
					if hdr == "" {
						s.logger.Debug("api: content length header not found")
						return
					}
				}
				contentLength, err := strconv.Atoi(hdr)
				if err != nil {
					s.logger.Debugf("api: content length: '%s', int conversion failed: %v", hdr, err)
					return
				}
				if contentLength > 0 {
					s.metrics.ContentApiDuration.WithLabelValues(fmt.Sprintf("%d", toFileSizeBucket(int64(contentLength))), r.Method).Observe(time.Since(now).Seconds())
				}
			case http.MethodPost:
				if r.ContentLength > 0 {
					s.metrics.ContentApiDuration.WithLabelValues(fmt.Sprintf("%d", toFileSizeBucket(r.ContentLength)), r.Method).Observe(time.Since(now).Seconds())
				}
			}
		})
	}
}

func lookaheadBufferSize(size int64) int {
	if size <= largeBufferFilesizeThreshold {
		return smallFileBufferSize
	}
	return largeFileBufferSize
}

// corsHandler sets CORS headers to HTTP response if allowed origins are configured.
func (s *Service) corsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && s.checkOrigin(r) {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "User-Agent, Origin, Accept, Authorization, Content-Type, X-Requested-With, Decompressed-Content-Length, Access-Control-Request-Headers, Access-Control-Request-Method, Swarm-Tag, Swarm-Pin, Swarm-Encrypt, Swarm-Index-Document, Swarm-Error-Document, Swarm-Collection, Swarm-Postage-Batch-Id, Gas-Price, Range, Accept-Ranges, Content-Encoding")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
		h.ServeHTTP(w, r)
	})
}

// checkOrigin returns true if the origin is not set or is equal to the request host.
func (s *Service) checkOrigin(r *http.Request) bool {
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

// newStamperPutter returns either a storingStamperPutter or a pushStamperPutter
// according to whether the upload is a deferred upload or not. in the case of
// direct push to the network (default) a pushStamperPutter is returned.
// returns a function to wait on the errorgroup in case of a pushing stamper putter.
func (s *Service) newStamperPutter(r *http.Request) (storage.Storer, func() error, error) {
	batch, err := requestPostageBatchId(r)
	if err != nil {
		return nil, noopWaitFn, fmt.Errorf("postage batch id: %w", err)
	}

	deferred, err := requestDeferred(r)
	if err != nil {
		return nil, noopWaitFn, fmt.Errorf("request deferred: %w", err)
	}

	exists, err := s.batchStore.Exists(batch)
	if err != nil {
		return nil, noopWaitFn, fmt.Errorf("batch exists: %w", err)
	}

	issuer, err := s.post.GetStampIssuer(batch)
	if err != nil {
		return nil, noopWaitFn, fmt.Errorf("stamp issuer: %w", err)
	}

	if usable := exists && s.post.IssuerUsable(issuer); !usable {
		return nil, noopWaitFn, errBatchUnusable
	}

	if deferred {
		p, err := newStoringStamperPutter(s.storer, s.post, s.signer, batch)
		return p, noopWaitFn, err
	}
	p, err := newPushStamperPutter(s.storer, s.post, s.signer, batch, s.chunkPushC)
	return p, p.eg.Wait, err
}

type pushStamperPutter struct {
	storage.Storer
	stamper postage.Stamper
	eg      errgroup.Group
	c       chan *pusher.Op
	sem     chan struct{}
}

func newPushStamperPutter(s storage.Storer, post postage.Service, signer crypto.Signer, batch []byte, cc chan *pusher.Op) (*pushStamperPutter, error) {
	i, err := post.GetStampIssuer(batch)
	if err != nil {
		return nil, fmt.Errorf("stamp issuer: %w", err)
	}

	stamper := postage.NewStamper(i, signer)
	return &pushStamperPutter{Storer: s, stamper: stamper, c: cc, sem: make(chan struct{}, uploadSem)}, nil
}

func (p *pushStamperPutter) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exists []bool, err error) {
	exists = make([]bool, len(chs))

	for i, c := range chs {
		// skips chunk we already know about
		has, err := p.Storer.Has(ctx, c.Address())
		if err != nil {
			return nil, err
		}
		if has || containsChunk(c.Address(), chs[:i]...) {
			exists[i] = true
			continue
		}
		stamp, err := p.stamper.Stamp(c.Address())
		if err != nil {
			return nil, err
		}

		func(ch swarm.Chunk) {
			p.sem <- struct{}{}
			p.eg.Go(func() error {
				defer func() {
					<-p.sem
				}()
				errc := make(chan error, 1)
				// note: shutdown might be tricky, we need to pass the quit channel
				// from the api here so that the putter knows not to keep on sending stuff
				// and just returns an error... or?
			PUSH:
				p.c <- &pusher.Op{Chunk: ch, Err: errc, Direct: true}
				select {
				case err := <-errc:
					// if we're the closest one we will store the chunk and return no error
					if errors.Is(err, topology.ErrWantSelf) {
						if _, err := p.Storer.Put(ctx, storage.ModePutSync, ch); err != nil {
							return err
						}
						return nil
					}
					if err == nil {
						return nil
					}
					goto PUSH
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}(c.WithStamp(stamp))
	}
	return exists, nil
}

type stamperPutter struct {
	storage.Storer
	stamper postage.Stamper
}

func newStoringStamperPutter(s storage.Storer, post postage.Service, signer crypto.Signer, batch []byte) (*stamperPutter, error) {
	i, err := post.GetStampIssuer(batch)
	if err != nil {
		return nil, fmt.Errorf("stamp issuer: %w", err)
	}

	stamper := postage.NewStamper(i, signer)
	return &stamperPutter{Storer: s, stamper: stamper}, nil
}

func (p *stamperPutter) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exists []bool, err error) {
	var (
		ctp []swarm.Chunk
		idx []int
	)
	exists = make([]bool, len(chs))

	for i, c := range chs {
		has, err := p.Storer.Has(ctx, c.Address())
		if err != nil {
			return nil, err
		}
		if has || containsChunk(c.Address(), chs[:i]...) {
			exists[i] = true
			continue
		}
		stamp, err := p.stamper.Stamp(c.Address())
		if err != nil {
			return nil, err
		}
		chs[i] = c.WithStamp(stamp)
		ctp = append(ctp, chs[i])
		idx = append(idx, i)
	}

	exists2, err := p.Storer.Put(ctx, mode, ctp...)
	if err != nil {
		return nil, err
	}
	for i, v := range idx {
		exists[v] = exists2[i]
	}
	return exists, nil
}

type pipelineFunc func(context.Context, io.Reader) (swarm.Address, error)

func requestPipelineFn(s storage.Putter, r *http.Request) pipelineFunc {
	mode, encrypt := requestModePut(r), requestEncrypt(r)
	return func(ctx context.Context, r io.Reader) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, mode, encrypt)
		return builder.FeedPipeline(ctx, pipe, r)
	}
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, r *http.Request) func() pipeline.Interface {
	mode, encrypt := requestModePut(r), requestEncrypt(r)
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, mode, encrypt)
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

// containsChunk returns true if the chunk with a specific address
// is present in the provided chunk slice.
func containsChunk(addr swarm.Address, chs ...swarm.Chunk) bool {
	for _, c := range chs {
		if addr.Equal(c.Address()) {
			return true
		}
	}
	return false
}

func noopWaitFn() error {
	return nil
}
