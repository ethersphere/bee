// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/swarm"
)

type (
	BytesPostResponse     = bytesPostResponse
	ChunkAddressResponse  = chunkAddressResponse
	SocPostResponse       = socPostResponse
	FeedReferenceResponse = feedReferenceResponse
	BzzUploadResponse     = bzzUploadResponse
	DebugTagResponse      = debugTagResponse
	TagRequest            = tagRequest
	ListTagsResponse      = listTagsResponse
	IsRetrievableResponse = isRetrievableResponse
	SecurityTokenResponse = securityTokenRsp
	SecurityTokenRequest  = securityTokenReq
)

var (
	InvalidContentType  = errInvalidContentType
	InvalidRequest      = errInvalidRequest
	DirectoryStoreError = errDirectoryStore
	EmptyDir            = errEmptyDir
)

var (
	ContentTypeTar    = contentTypeTar
	ContentTypeHeader = contentTypeHeader
)

var (
	ErrNoResolver                       = errNoResolver
	ErrInvalidNameOrAddress             = errInvalidNameOrAddress
	ErrUnsupportedDevNodeOperation      = errUnsupportedDevNodeOperation
	ErrOperationSupportedOnlyInFullMode = errOperationSupportedOnlyInFullMode
)

var (
	FeedMetadataEntryOwner = feedMetadataEntryOwner
	FeedMetadataEntryTopic = feedMetadataEntryTopic
	FeedMetadataEntryType  = feedMetadataEntryType

	SuccessWsMsg = successWsMsg
)

var (
	FileSizeBucketsKBytes = fileSizeBucketsKBytes
	ToFileSizeBucket      = toFileSizeBucket
)

func (s *Service) ResolveNameOrAddress(str string) (swarm.Address, error) {
	return s.resolveNameOrAddress(str)
}

func CalculateNumberOfChunks(contentLength int64, isEncrypted bool) int64 {
	return calculateNumberOfChunks(contentLength, isEncrypted)
}

type (
	HealthStatusResponse              = healthStatusResponse
	NodeResponse                      = nodeResponse
	PingpongResponse                  = pingpongResponse
	PeerConnectResponse               = peerConnectResponse
	PeersResponse                     = peersResponse
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
	GetStakeResponse                  = getStakeResponse
	WithdrawAllStakeResponse          = withdrawAllStakeResponse
	StatusSnapshotResponse            = statusSnapshotResponse
	StatusResponse                    = statusResponse
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

func MapStructure(input, output interface{}, hooks map[string]func(v string) (string, error)) error {
	return mapStructure(input, output, hooks)
}
func NewParseError(entry, value string, cause error) error {
	return newParseError(entry, value, cause)
}
