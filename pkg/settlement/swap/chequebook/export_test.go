// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package chequebook

import "time"

var (
	LastIssuedChequeKey        = lastIssuedChequeKey
	LastReceivedChequeKey      = lastReceivedChequeKey
	CashoutActionKey           = cashoutActionKey
	SetCoveringBalanceCachedAt = func(s Service, at time.Time) {
		svc := s.(*service)
		svc.lock.Lock()
		defer svc.lock.Unlock()
		svc.coveringBalanceAt = at
	}
	PendingWithdrawals = func(s Service) int {
		svc := s.(*service)
		svc.lock.Lock()
		defer svc.lock.Unlock()
		return len(svc.pendingWithdrawals)
	}
	SetWithdrawalReceiptHold = func(s Service, d time.Duration) {
		s.(*service).withdrawalReceiptHold = d
	}
	SetCoveringBalanceReadTimeout = func(s Service, d time.Duration) {
		s.(*service).coveringBalanceReadTimeout = d
	}
)
