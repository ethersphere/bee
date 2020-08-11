// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

type (
	StatusResponse           = statusResponse
	PingpongResponse         = pingpongResponse
	PeerConnectResponse      = peerConnectResponse
	PeersResponse            = peersResponse
	AddressesResponse        = addressesResponse
	PinnedChunk              = pinnedChunk
	ListPinnedChunksResponse = listPinnedChunksResponse
	WelcomeMessageRequest    = welcomeMessageRequest
	WelcomeMessageResponse   = welcomeMessageResponse
	BalancesResponse         = balancesResponse
	BalanceResponse          = balanceResponse
)

var (
	ErrCantBalance   = errCantBalance
	ErrCantBalances  = errCantBalances
	ErrInvaliAddress = errInvaliAddress
)
