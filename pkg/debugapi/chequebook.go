// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"math/big"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

var (
	errChequebookBalance = "Cannot get chequebook balance"
)

type chequebookBalanceResponse struct {
	Balance *big.Int `json:"balance"`
}

func (s *server) chequebookBalanceHandler(w http.ResponseWriter, r *http.Request) {
	balance, err := s.Chequebook.Balance(r.Context())
	if err != nil {
		jsonhttp.InternalServerError(w, errChequebookBalance)
		s.Logger.Debugf("debug api: chequebook balance: %v", err)
		s.Logger.Error("debug api: can not get chequebook balance")
		return
	}

	jsonhttp.OK(w, chequebookBalanceResponse{Balance: balance})
}
