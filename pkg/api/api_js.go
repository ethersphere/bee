//go:build js
// +build js

// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/accounting"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/gsoc"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/pingpong"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/pss"
	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/client/ens"
	"github.com/ethersphere/bee/v2/pkg/settlement"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/topology/lightnode"
	"github.com/ethersphere/bee/v2/pkg/tracing"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/semaphore"
)

type Service struct {
	storer          Storer
	resolver        resolver.Interface
	pss             pss.Interface
	gsoc            gsoc.Listener
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

	wsWg sync.WaitGroup // wait for all websockets to close on exit
	quit chan struct{}

	overlay           *swarm.Address
	publicKey         ecdsa.PublicKey
	pssPublicKey      ecdsa.PublicKey
	ethereumAddress   common.Address
	chequebookEnabled bool
	swapEnabled       bool
	fullAPIEnabled    bool

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
	isWarmingUp   bool
}

// Configure will create a and initialize a new API service.
func (s *Service) Configure(signer crypto.Signer, tracer *tracing.Tracer, o Options, e ExtraOptions, chainID int64, erc20 erc20.Service) {
	s.signer = signer
	s.Options = o
	s.tracer = tracer

	s.quit = make(chan struct{})

	s.storer = e.Storer
	s.resolver = e.Resolver
	s.pss = e.Pss
	s.gsoc = e.Gsoc
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
