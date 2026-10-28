// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
	dto "github.com/prometheus/client_model/go"
)

type (
	BytesPostResponse     = bytesPostResponse
	ChunkAddressResponse  = chunkAddressResponse
	SocPostResponse       = socPostResponse
	FeedReferenceResponse = feedReferenceResponse
	BzzUploadResponse     = bzzUploadResponse
	TagRequest            = tagRequest
	ListTagsResponse      = listTagsResponse
	IsRetrievableResponse = isRetrievableResponse
)

var (
	ErrInvalidContentType  = errInvalidContentType
	ErrInvalidRequest      = errInvalidRequest
	ErrDirectoryStoreError = errDirectoryStore
	ErrEmptyDir            = errEmptyDir
)

var ContentTypeTar = contentTypeTar

var (
	ErrNoResolver                       = errNoResolver
	ErrInvalidNameOrAddress             = errInvalidNameOrAddress
	ErrOperationSupportedOnlyInFullMode = errOperationSupportedOnlyInFullMode
	ErrActDownload                      = errActDownload
	ErrActUpload                        = errActUpload
)

var (
	FeedMetadataEntryOwner = feedMetadataEntryOwner
	FeedMetadataEntryTopic = feedMetadataEntryTopic
	FeedMetadataEntryType  = feedMetadataEntryType
)

const GsocQueueCapacity = gsocQueueCapacity

type GsocQueue = gsocQueue

func NewGsocQueue() *GsocQueue { return newGsocQueue() }

func (q *GsocQueue) Push(b []byte)       { q.push(b) }
func (q *GsocQueue) Pop() ([]byte, bool) { return q.pop() }
func (q *GsocQueue) Release()            { q.release() }

func (s *Service) CacheGsocWrappedChunks(address swarm.Address) func() {
	return s.cacheGsocWrappedChunks(address)
}

var (
	FileSizeBucketsKBytes = fileSizeBucketsKBytes
	ToFileSizeBucket      = toFileSizeBucket
	MetricsMethod         = metricsMethod
)

func (s *Service) ResolveNameOrAddress(str string) (swarm.Address, error) {
	return s.resolveNameOrAddress(str)
}

type (
	HealthStatusResponse              = healthStatusResponse
	NodeResponse                      = nodeResponse
	PingpongResponse                  = pingpongResponse
	PeerConnectResponse               = peerConnectResponse
	PeersResponse                     = peersResponse
	BlockedListedPeersResponse        = blockListedPeersResponse
	AddressesResponse                 = addressesResponse
	WelcomeMessageRequest             = welcomeMessageRequest
	WelcomeMessageResponse            = welcomeMessageResponse
	BalancesResponse                  = balancesResponse
	PeerDataResponse                  = peerDataResponse
	PeerData                          = peerData
	BalanceResponse                   = balanceResponse
	SettlementResponse                = settlementResponse
	SettlementsResponse               = settlementsResponse
	ChequebookBalanceResponse         = chequebookBalanceResponse
	ChequebookAddressResponse         = chequebookAddressResponse
	ChequebookLastChequePeerResponse  = chequebookLastChequePeerResponse
	ChequebookLastChequesResponse     = chequebookLastChequesResponse
	ChequebookLastChequesPeerResponse = chequebookLastChequesPeerResponse
	ChequebookTxResponse              = chequebookTxResponse
	SwapCashoutResponse               = swapCashoutResponse
	SwapCashoutStatusResponse         = swapCashoutStatusResponse
	SwapCashoutStatusResult           = swapCashoutStatusResult
	TransactionInfo                   = transactionInfo
	TransactionPendingList            = transactionPendingList
	TransactionHashResponse           = transactionHashResponse
	TagResponse                       = tagResponse
	ReserveStateResponse              = reserveStateResponse
	ChainStateResponse                = chainStateResponse
	PostageCreateResponse             = postageCreateResponse
	PostageStampResponse              = postageStampResponse
	PostageStampsResponse             = postageStampsResponse
	PostageBatchResponse              = postageBatchResponse
	PostageStampBucketsResponse       = postageStampBucketsResponse
	BucketData                        = bucketData
	WalletResponse                    = walletResponse
	WalletTxResponse                  = walletTxResponse
	GetStakeResponse                  = getStakeResponse
	StakeDepositErrorResponse         = stakeDepositErrorResponse
	GetWithdrawableResponse           = getWithdrawableResponse
	StakeTransactionReponse           = stakeTransactionReponse
	StatusSnapshotResponse            = statusSnapshotResponse
	StatusResponse                    = statusResponse
	RedistributionStatusResponse      = redistributionStatusResponse
	RedistributionToggleResponse      = redistributionToggleResponse
)

var (
	ErrCantBalance           = errCantBalance
	ErrCantBalances          = errCantBalances
	HttpErrGetAccountingInfo = httpErrGetAccountingInfo
	ErrNoBalance             = errNoBalance
	ErrCantSettlementsPeer   = errCantSettlementsPeer
	ErrCantSettlements       = errCantSettlements
	ErrChequebookBalance     = errChequebookBalance
	ErrInvalidAddress        = errInvalidAddress
	ErrUnknownTransaction    = errUnknownTransaction
	ErrCantGetTransaction    = errCantGetTransaction
	ErrCantResendTransaction = errCantResendTransaction
	ErrAlreadyImported       = errAlreadyImported
)

type (
	LogRegistryIterateFn   func(fn func(string, string, log.Level, uint) bool)
	LogSetVerbosityByExpFn func(e string, v log.Level) error
)

var (
	LogRegistryIterate   = logRegistryIterate
	LogSetVerbosityByExp = logSetVerbosityByExp
)

func ReplaceLogRegistryIterateFn(fn LogRegistryIterateFn)   { logRegistryIterate = fn }
func ReplaceLogSetVerbosityByExp(fn LogSetVerbosityByExpFn) { logSetVerbosityByExp = fn }

var ErrHexLength = errHexLength

type HexInvalidByteError = hexInvalidByteError

func MapStructure(input, output any, hooks map[string]func(v string) (string, error)) error {
	return mapStructure(input, output, hooks)
}

func NewParseError(entry, value string, cause error) error {
	return newParseError(entry, value, cause)
}

const (
	MaxStreamQueueSize      = maxStreamQueueSize
	DefaultStreamSubWorkers = defaultStreamSubWorkers
	MaxStreamFrameSize      = maxStreamFrameSize
)

// ChunkStreamDeliveryCount returns the chunk stream delivery counter for one
// outcome.
func (s *Service) ChunkStreamDeliveryCount(status string) float64 {
	m := &dto.Metric{}
	if err := s.metrics.ChunkStreamDeliveryCount.WithLabelValues(status).Write(m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}

// ChunkStreamOpenConnections returns the number of open chunk streams.
func (s *Service) ChunkStreamOpenConnections() float64 {
	m := &dto.Metric{}
	if err := s.metrics.ChunkStreamOpenConnections.WithLabelValues("stream").Write(m); err != nil {
		return -1
	}
	return m.GetGauge().GetValue()
}

const RootPath = rootPath

func (s *Service) Router() *mux.Router { return s.router }
