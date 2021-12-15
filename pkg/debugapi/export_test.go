// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

type (
	StatusResponse                    = statusResponse
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
	PostageStampBucketsResponse       = postageStampBucketsResponse
	BucketData                        = bucketData
)

var (
	ErrCantBalance           = errCantBalance
	ErrCantBalances          = errCantBalances
	ErrCantInfo              = errCantInfo
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
