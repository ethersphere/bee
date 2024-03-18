// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
)

const (
	httpErrGetAccountingInfo = "Cannot get accounting info"
)

type peerData struct {
	InfoResponse map[string]peerDataResponse `json:"peerData"`
}

type peerDataResponse struct {
	Balance                  *bigint.BigInt `json:"balance"`
	ConsumedBalance          *bigint.BigInt `json:"consumedBalance"`
	ThresholdReceived        *bigint.BigInt `json:"thresholdReceived"`
	ThresholdGiven           *bigint.BigInt `json:"thresholdGiven"`
	CurrentThresholdReceived *bigint.BigInt `json:"currentThresholdReceived"`
	CurrentThresholdGiven    *bigint.BigInt `json:"currentThresholdGiven"`
	SurplusBalance           *bigint.BigInt `json:"surplusBalance"`
	ReservedBalance          *bigint.BigInt `json:"reservedBalance"`
	ShadowReservedBalance    *bigint.BigInt `json:"shadowReservedBalance"`
	GhostBalance             *bigint.BigInt `json:"ghostBalance"`
}

func (s *Service) accountingInfoHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_accounting").Build()

	infos, err := s.accounting.PeerAccounting()
	if err != nil {
		jsonhttp.InternalServerError(w, httpErrGetAccountingInfo)
		logger.Debug("accounting info failed to load balances")
		logger.Error(err, "can not get accounting info")
		return
	}

	infoResponses := make(map[string]peerDataResponse, len(infos))
	for k := range infos {
		infoResponses[k] = peerDataResponse{
			Balance:                  bigint.Wrap(infos[k].Balance),
			ConsumedBalance:          bigint.Wrap(infos[k].ConsumedBalance),
			ThresholdReceived:        bigint.Wrap(infos[k].ThresholdReceived),
			ThresholdGiven:           bigint.Wrap(infos[k].ThresholdGiven),
			CurrentThresholdReceived: bigint.Wrap(infos[k].CurrentThresholdReceived),
			CurrentThresholdGiven:    bigint.Wrap(infos[k].CurrentThresholdGiven),
			SurplusBalance:           bigint.Wrap(infos[k].SurplusBalance),
			ReservedBalance:          bigint.Wrap(infos[k].ReservedBalance),
			ShadowReservedBalance:    bigint.Wrap(infos[k].ShadowReservedBalance),
			GhostBalance:             bigint.Wrap(infos[k].GhostBalance),
		}
	}

	jsonhttp.OK(w, peerData{InfoResponse: infoResponses})
}
