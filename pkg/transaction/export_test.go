// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"math/big"
)

var StoredTransactionKey = storedTransactionKey

func SetSuggestFees(s Service, f func(context.Context, TxOptions) (*big.Int, *big.Int, error)) {
	s.(*transactionService).suggestFees = f
}
