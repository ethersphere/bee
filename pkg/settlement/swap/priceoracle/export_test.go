// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package priceoracle

import "math/big"

// SetRates drives the poll loop's rate-update path for tests, so the write can
// be exercised concurrently with CurrentRates under the race detector.
func SetRates(s Service, exchangeRate, deduction *big.Int) {
	s.(*service).setRates(exchangeRate, deduction)
}
