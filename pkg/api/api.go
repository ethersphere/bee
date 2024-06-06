// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api provides the functionality of the Bee
// client-facing HTTP API.
package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/auth"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/steward"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/hashicorp/go-multierror"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "api"

const (
	SwarmPinHeader                    = "Swarm-Pin"
	SwarmTagHeader                    = "Swarm-Tag"
	SwarmEncryptHeader                = "Swarm-Encrypt"
	SwarmIndexDocumentHeader          = "Swarm-Index-Document"
	SwarmErrorDocumentHeader          = "Swarm-Error-Document"
	SwarmFeedIndexHeader              = "Swarm-Feed-Index"
	SwarmFeedIndexNextHeader          = "Swarm-Feed-Index-Next"
	SwarmCollectionHeader             = "Swarm-Collection"
	SwarmPostageBatchIdHeader         = "Swarm-Postage-Batch-Id"
	SwarmDeferredUploadHeader         = "Swarm-Deferred-Upload"
	SwarmRedundancyLevelHeader        = "Swarm-Redundancy-Level"
	SwarmRedundancyStrategyHeader     = "Swarm-Redundancy-Strategy"
	SwarmRedundancyFallbackModeHeader = "Swarm-Redundancy-Fallback-Mode"
	SwarmChunkRetrievalTimeoutHeader  = "Swarm-Chunk-Retrieval-Timeout"
	SwarmLookAheadBufferSizeHeader    = "Swarm-Lookahead-Buffer-Size"
	SwarmActHeader                    = "Swarm-Act"
	SwarmActTimestampHeader           = "Swarm-Act-Timestamp"
	SwarmActPublisherHeader           = "Swarm-Act-Publisher"
	SwarmActHistoryAddressHeader      = "Swarm-Act-History-Address"

	ImmutableHeader = "Immutable"
	GasPriceHeader  = "Gas-Price"
	GasLimitHeader  = "Gas-Limit"
	ETagHeader      = "ETag"

	AuthorizationHeader      = "Authorization"
	AcceptEncodingHeader     = "Accept-Encoding"
	ContentTypeHeader        = "Content-Type"
	ContentDispositionHeader = "Content-Disposition"
	ContentLengthHeader      = "Content-Length"
	RangeHeader              = "Range"
	OriginHeader             = "Origin"
)

const (
	multiPartFormData  = "multipart/form-data"
	contentTypeTar     = "application/x-tar"
	boolHeaderSetValue = "true"
)

var (
	errInvalidNameOrAddress             = errors.New("invalid name or bzz address")
	errNoResolver                       = errors.New("no resolver connected")
	errInvalidRequest                   = errors.New("could not validate request")
	errInvalidContentType               = errors.New("invalid content-type")
	errDirectoryStore                   = errors.New("could not store directory")
	errFileStore                        = errors.New("could not store file")
	errInvalidPostageBatch              = errors.New("invalid postage batch id")
	errBatchUnusable                    = errors.New("batch not usable")
	errUnsupportedDevNodeOperation      = errors.New("operation not supported in dev mode")
	errOperationSupportedOnlyInFullMode = errors.New("operation is supported only in full mode")
	errActDownload                      = errors.New("act download failed")
	errActUpload                        = errors.New("act upload failed")
)

// Storer interface provides the functionality required from the local storage
// component of the node.
type Storer interface {
	storer.UploadStore
	storer.PinStore
	storer.CacheStore
	storer.NetStore
	storer.LocalStore
	storer.RadiusChecker
	storer.Debugger
}

type PinIntegrity interface {
	Check(ctx context.Context, logger log.Logger, pin string, out chan storer.PinStat)
}

type Service struct {
	auth            auth.Authenticator
	storer          Storer
	resolver        resolver.Interface
	pss             pss.Interface
	steward         steward.Interface
	logger          log.Logger
	loggerV1        log.Logger
	tracer          *tracing.Tracer
	feedFactory     feeds.Factory
	signer          crypto.Signer
	post            postage.Service
	accesscontrol   accesscontrol.Controller
	postageContract postagecontract.Interface
	probe           *Probe
	metricsRegistry *prometheus.Registry
	stakingContract staking.Contract
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

	batchStore   postage.Storer
	stamperStore storage.Store
	pinIntegrity PinIntegrity

	syncStatus func() (bool, error)

	swap        swap.Interface
	transaction transaction.Service
	lightNodes  *lightnode.Container
	blockTime   time.Duration

	statusSem        *semaphore.Weighted
	postageSem       *semaphore.Weighted
	stakingSem       *semaphore.Weighted
	cashOutChequeSem *semaphore.Weighted
	beeMode          BeeNodeMode

	chainBackend transaction.Backend
	erc20Service erc20.Service
	chainID      int64

	whitelistedWithdrawalAddress []common.Address

	preMapHooks map[string]func(v string) (string, error)
	validate    *validator.Validate

	redistributionAgent *storageincentives.Agent

	statusService *status.Service
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

func (s *Service) SetRedistributionAgent(redistributionAgent *storageincentives.Agent) {
	if s != nil {
		s.redistributionAgent = redistributionAgent
	}
}

type Options struct {
	CORSAllowedOrigins []string
	WsPingPeriod       time.Duration
	Restricted         bool
}

type ExtraOptions struct {
	Pingpong        pingpong.Interface
	TopologyDriver  topology.Driver
	LightNodes      *lightnode.Container
	Accounting      accounting.Interface
	Pseudosettle    settlement.Interface
	Swap            swap.Interface
	Chequebook      chequebook.Service
	BlockTime       time.Duration
	Storer          Storer
	Resolver        resolver.Interface
	Pss             pss.Interface
	FeedFactory     feeds.Factory
	Post            postage.Service
	AccessControl   accesscontrol.Controller
	PostageContract postagecontract.Interface
	Staking         staking.Contract
	Steward         steward.Interface
	SyncStatus      func() (bool, error)
	NodeStatus      *status.Service
	PinIntegrity    PinIntegrity
}

func New(
	publicKey, pssPublicKey ecdsa.PublicKey,
	ethereumAddress common.Address,
	whitelistedWithdrawalAddress []string,
	logger log.Logger,
	transaction transaction.Service,
	batchStore postage.Storer,
	beeMode BeeNodeMode,
	chequebookEnabled bool,
	swapEnabled bool,
	chainBackend transaction.Backend,
	cors []string,
	stamperStore storage.Store,
) *Service {
	s := new(Service)

	s.CORSAllowedOrigins = cors
	s.beeMode = beeMode
	s.logger = logger.WithName(loggerName).Register()
	s.loggerV1 = s.logger.V(1).Register()
	s.chequebookEnabled = chequebookEnabled
	s.swapEnabled = swapEnabled
	s.publicKey = publicKey
	s.pssPublicKey = pssPublicKey
	s.ethereumAddress = ethereumAddress
	s.transaction = transaction
	s.batchStore = batchStore
	s.chainBackend = chainBackend
	s.metricsRegistry = newDebugMetrics()
	s.preMapHooks = map[string]func(v string) (string, error){
		"mimeMediaType": func(v string) (string, error) {
			typ, _, err := mime.ParseMediaType(v)
			return typ, err
		},
		"decBase64url": func(v string) (string, error) {
			buf, err := base64.URLEncoding.DecodeString(v)
			return string(buf), err
		},
		"decHex": func(v string) (string, error) {
			buf, err := hex.DecodeString(v)
			return string(buf), err
		},
	}
	s.validate = validator.New()
	s.validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get(mapStructureTagName), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	s.stamperStore = stamperStore

	for _, v := range whitelistedWithdrawalAddress {
		s.whitelistedWithdrawalAddress = append(s.whitelistedWithdrawalAddress, common.HexToAddress(v))
	}

	return s
}

// Configure will create a and initialize a new API service.
func (s *Service) Configure(signer crypto.Signer, auth auth.Authenticator, tracer *tracing.Tracer, o Options, e ExtraOptions, chainID int64, erc20 erc20.Service) {
	s.auth = auth
	s.signer = signer
	s.Options = o
	s.tracer = tracer
	s.metrics = newMetrics()

	s.quit = make(chan struct{})

	s.storer = e.Storer
	s.resolver = e.Resolver
	s.pss = e.Pss
	s.feedFactory = e.FeedFactory
	s.post = e.Post
	s.accesscontrol = e.AccessControl
	s.postageContract = e.PostageContract
	s.steward = e.Steward
	s.stakingContract = e.Staking

	s.pingpong = e.Pingpong
	s.topologyDriver = e.TopologyDriver
	s.accounting = e.Accounting
	s.chequebook = e.Chequebook
	s.swap = e.Swap
	s.lightNodes = e.LightNodes
	s.pseudosettle = e.Pseudosettle
	s.blockTime = e.BlockTime

	s.statusSem = semaphore.NewWeighted(1)
	s.postageSem = semaphore.NewWeighted(1)
	s.stakingSem = semaphore.NewWeighted(1)
	s.cashOutChequeSem = semaphore.NewWeighted(1)

	s.chainID = chainID
	s.erc20Service = erc20
	s.syncStatus = e.SyncStatus

	s.statusService = e.NodeStatus

	s.preMapHooks["resolve"] = func(v string) (string, error) {
		switch addr, err := s.resolveNameOrAddress(v); {
		case err == nil:
			return addr.String(), nil
		case errors.Is(err, ens.ErrNotImplemented):
			return v, nil
		default:
			return "", err
		}
	}

	s.pinIntegrity = e.PinIntegrity
}

func (s *Service) SetProbe(probe *Probe) {
	s.probe = probe
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

// getOrCreateSessionID attempts to get the session if an tag id is supplied, and returns an error
// if it does not exist. If no id is supplied, it will attempt to create a new session and return it.
func (s *Service) getOrCreateSessionID(tagUid uint64) (uint64, error) {
	var (
		tag storer.SessionInfo
		err error
	)
	// if tag ID is not supplied, create a new tag
	if tagUid == 0 {
		tag, err = s.storer.NewSession()
	} else {
		tag, err = s.storer.Session(tagUid)
	}
	return tag.TagID, err
}

func (s *Service) resolveNameOrAddress(str string) (swarm.Address, error) {
	// Try and mapStructure the name as a bzz address.
	addr, err := swarm.ParseHexAddress(str)
	if err == nil {
		s.loggerV1.Debug("resolve name: parsing bzz address successful", "string", str, "address", addr)
		return addr, nil
	}

	// If no resolver is not available, return an error.
	if s.resolver == nil {
		return swarm.ZeroAddress, errNoResolver
	}

	// Try and resolve the name using the provided resolver.
	s.logger.Debug("resolve name: attempting to resolve string to address", "string", str)
	addr, err = s.resolver.Resolve(str)
	if err == nil {
		s.loggerV1.Debug("resolve name: address resolved successfully", "string", str, "address", addr)
		return addr, nil
	}

	return swarm.ZeroAddress, fmt.Errorf("%w: %w", errInvalidNameOrAddress, err)
}

type securityTokenRsp struct {
	Key string `json:"key"`
}

type securityTokenReq struct {
	Role   string `json:"role"`
	Expiry int    `json:"expiry"` // duration in seconds
}

func (s *Service) authHandler(w http.ResponseWriter, r *http.Request) {
	_, pass, ok := r.BasicAuth()

	if !ok {
		s.logger.Error(nil, "auth handler: missing basic auth")
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		jsonhttp.Unauthorized(w, "Unauthorized")
		return
	}

	if !s.auth.Authorize(pass) {
		s.logger.Error(nil, "auth handler: unauthorized")
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		jsonhttp.Unauthorized(w, "Unauthorized")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Debug("auth handler: read request body failed", "error", err)
		s.logger.Error(nil, "auth handler: read request body failed")
		jsonhttp.BadRequest(w, "Read request body")
		return
	}

	var payload securityTokenReq
	if err = json.Unmarshal(body, &payload); err != nil {
		s.logger.Debug("auth handler: unmarshal request body failed", "error", err)
		s.logger.Error(nil, "auth handler: unmarshal request body failed")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	key, err := s.auth.GenerateKey(payload.Role, time.Duration(payload.Expiry)*time.Second)
	if errors.Is(err, auth.ErrExpiry) {
		s.logger.Debug("auth handler: generate key failed", "error", err)
		s.logger.Error(nil, "auth handler: generate key failed")
		jsonhttp.BadRequest(w, "Expiry duration must be a positive number")
		return
	}
	if err != nil {
		s.logger.Debug("auth handler: add auth token failed", "error", err)
		s.logger.Error(nil, "auth handler: add auth token failed")
		jsonhttp.InternalServerError(w, "Error generating authorization token")
		return
	}

	jsonhttp.Created(w, securityTokenRsp{
		Key: key,
	})
}

func (s *Service) refreshHandler(w http.ResponseWriter, r *http.Request) {
	reqToken := r.Header.Get(AuthorizationHeader)
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

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Debug("auth handler: read request body failed", "error", err)
		s.logger.Error(nil, "auth handler: read request body failed")
		jsonhttp.BadRequest(w, "Read request body")
		return
	}

	var payload securityTokenReq
	if err = json.Unmarshal(body, &payload); err != nil {
		s.logger.Debug("auth handler: unmarshal request body failed", "error", err)
		s.logger.Error(nil, "auth handler: unmarshal request body failed")
		jsonhttp.BadRequest(w, "Unmarshal json body")
		return
	}

	key, err := s.auth.RefreshKey(authToken, time.Duration(payload.Expiry)*time.Second)
	if errors.Is(err, auth.ErrTokenExpired) {
		s.logger.Debug("auth handler: refresh key failed", "error", err)
		s.logger.Error(nil, "auth handler: refresh key failed")
		jsonhttp.BadRequest(w, "Token expired")
		return
	}

	if err != nil {
		s.logger.Debug("auth handler: refresh token failed", "error", err)
		s.logger.Error(nil, "auth handler: refresh token failed")
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
				s.logger.Debug("extract tracing context failed", "span_name", spanName, "error", err)
				// ignore
			}

			span, _, ctx := s.tracer.StartSpanFromContext(ctx, spanName, s.logger)
			defer span.Finish()

			err = s.tracer.AddContextHTTPHeader(ctx, r.Header)
			if err != nil {
				s.logger.Debug("inject tracing context failed", "span_name", spanName, "error", err)
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
				hdr := w.Header().Get(ContentLengthHeader)
				if hdr == "" {
					s.logger.Debug("content length header not found")
					return
				}

				contentLength, err := strconv.Atoi(hdr)
				if err != nil {
					s.logger.Debug("int conversion failed", "content_length", hdr, "error", err)
					return
				}
				if contentLength > 0 {
					s.metrics.ContentApiDuration.WithLabelValues(strconv.FormatInt(toFileSizeBucket(int64(contentLength)), 10), r.Method).Observe(time.Since(now).Seconds())
				}
			case http.MethodPost:
				if r.ContentLength > 0 {
					s.metrics.ContentApiDuration.WithLabelValues(strconv.FormatInt(toFileSizeBucket(r.ContentLength), 10), r.Method).Observe(time.Since(now).Seconds())
				}
			}
		})
	}
}

// gasConfigMiddleware can be used by the APIs that allow block chain transactions to set
// gas price and gas limit through the HTTP API headers.
func (s *Service) gasConfigMiddleware(handlerName string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := s.logger.WithName(handlerName).Build()

			headers := struct {
				GasPrice *big.Int `map:"Gas-Price"`
				GasLimit uint64   `map:"Gas-Limit"`
			}{}
			if response := s.mapStructure(r.Header, &headers); response != nil {
				response("invalid header params", logger, w)
				return
			}
			ctx := r.Context()
			ctx = sctx.SetGasPrice(ctx, headers.GasPrice)
			ctx = sctx.SetGasLimit(ctx, headers.GasLimit)

			h.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// corsHandler sets CORS headers to HTTP response if allowed origins are configured.
func (s *Service) corsHandler(h http.Handler) http.Handler {
	allowedHeaders := []string{
		"User-Agent", "Accept", "X-Requested-With", "Access-Control-Request-Headers", "Access-Control-Request-Method", "Accept-Ranges", "Content-Encoding",
		AuthorizationHeader, AcceptEncodingHeader, ContentTypeHeader, ContentDispositionHeader, RangeHeader, OriginHeader,
		SwarmTagHeader, SwarmPinHeader, SwarmEncryptHeader, SwarmIndexDocumentHeader, SwarmErrorDocumentHeader, SwarmCollectionHeader, SwarmPostageBatchIdHeader, SwarmDeferredUploadHeader, SwarmRedundancyLevelHeader, SwarmRedundancyStrategyHeader, SwarmRedundancyFallbackModeHeader, SwarmChunkRetrievalTimeoutHeader, SwarmLookAheadBufferSizeHeader, SwarmFeedIndexHeader, SwarmFeedIndexNextHeader, GasPriceHeader, GasLimitHeader, ImmutableHeader,
	}
	allowedHeadersStr := strings.Join(allowedHeaders, ", ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get(OriginHeader); o != "" && s.checkOrigin(r) {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", allowedHeadersStr)
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
		h.ServeHTTP(w, r)
	})
}

// checkOrigin returns true if the origin is not set or is equal to the request host.
func (s *Service) checkOrigin(r *http.Request) bool {
	origin := r.Header[OriginHeader]
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

// validationError is a custom error type for validation errors.
type validationError struct {
	Entry string
	Value interface{}
	Cause error
}

// Error implements the error interface.
func (e *validationError) Error() string {
	return fmt.Sprintf("`%s=%v`: %v", e.Entry, e.Value, e.Cause)
}

// mapStructure maps the input into output struct and validates the output.
// It's a helper method for the handlers, which reduces the chattiness
// of the code.
func (s *Service) mapStructure(input, output interface{}) func(string, log.Logger, http.ResponseWriter) {
	// response unifies the response format for parsing and validation errors.
	response := func(err error) func(string, log.Logger, http.ResponseWriter) {
		return func(msg string, logger log.Logger, w http.ResponseWriter) {
			var merr *multierror.Error
			if !errors.As(err, &merr) {
				logger.Debug("mapping and validation failed", "error", err)
				logger.Error(err, "mapping and validation failed")
				jsonhttp.InternalServerError(w, err)
				return
			}

			logger.Debug(msg, "error", err)
			logger.Error(err, msg)

			resp := jsonhttp.StatusResponse{
				Message: msg,
				Code:    http.StatusBadRequest,
			}
			for _, err := range merr.Errors {
				var perr *parseError
				if errors.As(err, &perr) {
					resp.Reasons = append(resp.Reasons, jsonhttp.Reason{
						Field: perr.Entry,
						Error: perr.Cause.Error(),
					})
				}
				var verr *validationError
				if errors.As(err, &verr) {
					resp.Reasons = append(resp.Reasons, jsonhttp.Reason{
						Field: verr.Entry,
						Error: verr.Cause.Error(),
					})
				}
			}
			jsonhttp.BadRequest(w, resp)
		}
	}

	if err := mapStructure(input, output, s.preMapHooks); err != nil {
		return response(err)
	}

	if err := s.validate.Struct(output); err != nil {
		var errs validator.ValidationErrors
		if !errors.As(err, &errs) {
			return response(err)
		}

		vErrs := &multierror.Error{ErrorFormat: flattenErrorsFormat}
		for _, err := range errs {
			val := err.Value()
			switch v := err.Value().(type) {
			case []byte:
				val = string(v)
			}
			vErrs = multierror.Append(vErrs,
				&validationError{
					Entry: strings.ToLower(err.Field()),
					Value: val,
					Cause: fmt.Errorf("want %s:%s", err.Tag(), err.Param()),
				})
		}
		return response(vErrs.ErrorOrNil())
	}

	return nil
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

type putterOptions struct {
	BatchID  []byte
	TagID    uint64
	Deferred bool
	Pin      bool
}

type putterSessionWrapper struct {
	storer.PutterSession
	stamper postage.Stamper
	save    func() error
}

func (p *putterSessionWrapper) Put(ctx context.Context, chunk swarm.Chunk) error {
	stamp, err := p.stamper.Stamp(chunk.Address())
	if err != nil {
		return err
	}
	return p.PutterSession.Put(ctx, chunk.WithStamp(stamp))
}

func (p *putterSessionWrapper) Done(ref swarm.Address) error {
	err := p.PutterSession.Done(ref)
	if err != nil {
		return err
	}
	return p.save()
}

func (p *putterSessionWrapper) Cleanup() error {
	return p.PutterSession.Cleanup()
}

func (s *Service) getStamper(batchID []byte) (postage.Stamper, func() error, error) {
	exists, err := s.batchStore.Exists(batchID)
	if err != nil {
		return nil, nil, fmt.Errorf("batch exists: %w", err)
	}

	issuer, save, err := s.post.GetStampIssuer(batchID)
	if err != nil {
		return nil, nil, fmt.Errorf("stamp issuer: %w", err)
	}

	if usable := exists && s.post.IssuerUsable(issuer); !usable {
		return nil, nil, errBatchUnusable
	}

	return postage.NewStamper(s.stamperStore, issuer, s.signer), save, nil
}

func (s *Service) newStamperPutter(ctx context.Context, opts putterOptions) (storer.PutterSession, error) {
	if !opts.Deferred && s.beeMode == DevMode {
		return nil, errUnsupportedDevNodeOperation
	}

	stamper, save, err := s.getStamper(opts.BatchID)
	if err != nil {
		return nil, fmt.Errorf("get stamper: %w", err)
	}

	var session storer.PutterSession
	if opts.Deferred || opts.Pin {
		session, err = s.storer.Upload(ctx, opts.Pin, opts.TagID)
	} else {
		session = s.storer.DirectUpload()
	}

	if err != nil {
		return nil, fmt.Errorf("failed creating session: %w", err)
	}

	return &putterSessionWrapper{
		PutterSession: session,
		stamper:       stamper,
		save:          save,
	}, nil
}

type pipelineFunc func(context.Context, io.Reader) (swarm.Address, error)

func requestPipelineFn(s storage.Putter, encrypt bool, rLevel redundancy.Level) pipelineFunc {
	return func(ctx context.Context, r io.Reader) (swarm.Address, error) {
		pipe := builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
		return builder.FeedPipeline(ctx, pipe, r)
	}
}

func requestPipelineFactory(ctx context.Context, s storage.Putter, encrypt bool, rLevel redundancy.Level) func() pipeline.Interface {
	return func() pipeline.Interface {
		return builder.NewPipelineBuilder(ctx, s, encrypt, rLevel)
	}
}

type cleanupOnErrWriter struct {
	http.ResponseWriter
	logger log.Logger
	onErr  func() error
}

func (r *cleanupOnErrWriter) WriteHeader(statusCode int) {
	// if there is an error status returned, cleanup.
	if statusCode >= http.StatusBadRequest {
		err := r.onErr()
		if err != nil {
			r.logger.Debug("failed cleaning up", "err", err)
		}
	}
	r.ResponseWriter.WriteHeader(statusCode)
}

// CalculateNumberOfChunks calculates the number of chunks in an arbitrary
// content length.
func CalculateNumberOfChunks(contentLength int64, isEncrypted bool) int64 {
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

// defaultUploadMethod returns true for deferred when the defered header is not present.
func defaultUploadMethod(deffered *bool) bool {
	if deffered == nil {
		return true
	}

	return *deffered
}
