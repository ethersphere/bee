// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
)

var (
	errCantInfo = "Cannot get accounting infos"
)

type infoResponseArray struct {
	infoResponse []accountingInfoResponse `json:"infos"`
}

type accountingInfoResponse struct {
	Peer                  string         `json:"peer"`
	Balance               *bigint.BigInt `json:"balance"`
	ThresholdReceived     *bigint.BigInt `json:"thresholdReceived"`
	ThresholdGiven        *bigint.BigInt `json:"thresholdGiven"`
	SurplusBalance        *bigint.BigInt `json:"surplusBalance"`
	ReservedBalance       *bigint.BigInt `json:"reservedBalance"`
	ShadowReservedBalance *bigint.BigInt `json:"shadowReservedBalance"`
	GhostBalance          *bigint.BigInt `json:"ghostBalance"`
}

func (s *Service) accountingInfoHandler(w http.ResponseWriter, r *http.Request) {
	infos, err := s.accounting.AccountingInfo()
	if err != nil {
		jsonhttp.InternalServerError(w, errCantInfo)
		s.logger.Debugf("debug api: accounting info: %v", err)
		s.logger.Error("debug api: can not get accounting info")
		return
	}

	infoResponses := make([]accountingInfoResponse, len(infos))
	i := 0
	for k := range infos {
		infoResponses[i] = accountingInfoResponse{
			Peer:                  infos[k].Peer,
			Balance:               bigint.Wrap(infos[k].Balance),
			ThresholdReceived:     bigint.Wrap(infos[k].ThresholdReceived),
			ThresholdGiven:        bigint.Wrap(infos[k].ThresholdGiven),
			SurplusBalance:        bigint.Wrap(infos[k].SurplusBalance),
			ReservedBalance:       bigint.Wrap(infos[k].ReservedBalance),
			ShadowReservedBalance: bigint.Wrap(infos[k].ShadowReservedBalance),
			GhostBalance:          bigint.Wrap(infos[k].GhostBalance),
		}
		i++
	}

	jsonhttp.OK(w, infoResponseArray{infoResponse: infoResponses})
}
