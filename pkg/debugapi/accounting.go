// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

const (
	httpErrGetAccountingInfo = "Cannot get accounting info"
)

type peerData struct {
	InfoResponse map[string]peerDataResponse `json:"peerData"`
}

type peerDataResponse struct {
	Balance               *bigint.BigInt `json:"balance"`
	ThresholdReceived     *bigint.BigInt `json:"thresholdReceived"`
	ThresholdGiven        *bigint.BigInt `json:"thresholdGiven"`
	SurplusBalance        *bigint.BigInt `json:"surplusBalance"`
	ReservedBalance       *bigint.BigInt `json:"reservedBalance"`
	ShadowReservedBalance *bigint.BigInt `json:"shadowReservedBalance"`
	GhostBalance          *bigint.BigInt `json:"ghostBalance"`
}

func (s *Service) accountingInfoHandler(w http.ResponseWriter, r *http.Request) {
	infos, err := s.accounting.PeerAccounting()
	if err != nil {
		jsonhttp.InternalServerError(w, httpErrGetAccountingInfo)
		s.logger.Debugf("debug api: accounting info: %v", err)
		s.logger.Error("debug api: can not get accounting info")
		return
	}

	infoResponses := make(map[string]peerDataResponse, len(infos))
	for k := range infos {
		infoResponses[k] = peerDataResponse{
			Balance:               bigint.Wrap(infos[k].Balance),
			ThresholdReceived:     bigint.Wrap(infos[k].ThresholdReceived),
			ThresholdGiven:        bigint.Wrap(infos[k].ThresholdGiven),
			SurplusBalance:        bigint.Wrap(infos[k].SurplusBalance),
			ReservedBalance:       bigint.Wrap(infos[k].ReservedBalance),
			ShadowReservedBalance: bigint.Wrap(infos[k].ShadowReservedBalance),
			GhostBalance:          bigint.Wrap(infos[k].GhostBalance),
		}
	}

	jsonhttp.OK(w, peerData{InfoResponse: infoResponses})
}
