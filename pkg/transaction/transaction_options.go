// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"errors"
	"fmt"
	"math/big"
)

var ErrFeePriorityWithGasPrice = errors.New("fee priority cannot be combined with gas price")

// FeeOptions selects how a transaction is priced.
// Priority and GasPrice cannot be combined.
// A set Priority prices the transaction from eth_feeHistory.
// A set GasPrice is a fixed price. Otherwise the suggested tip is raised by TipBoostPercent.
type FeeOptions struct {
	GasPrice        *big.Int
	Priority        int
	TipBoostPercent int
}

func (o FeeOptions) Validate() error {
	if o.TipBoostPercent < 0 {
		return fmt.Errorf("negative boostPercent (%d) not allowed", o.TipBoostPercent)
	}
	if o.Priority != 0 {
		if err := ValidateFeePriority(o.Priority); err != nil {
			return err
		}
		if o.GasPrice != nil {
			return ErrFeePriorityWithGasPrice
		}
	}
	return nil
}

func resolveTxOptions(opts ...Option) (TxOptions, error) {
	resolved := Apply(opts...)
	if err := resolved.Fee.Validate(); err != nil {
		return TxOptions{}, err
	}
	return resolved, nil
}

type Option func(*TxOptions)

type TxOptions struct {
	Fee      FeeOptions
	GasLimit uint64 // zero leaves the gas limit to the call site
}

func WithGasPrice(price *big.Int) Option {
	return func(o *TxOptions) {
		o.Fee.GasPrice = price
	}
}

func WithGasLimit(limit uint64) Option {
	return func(o *TxOptions) {
		o.GasLimit = limit
	}
}

// WithFixedGasLimitAtLeast raises the gas limit to limit when it is lower.
// A non-zero gas limit is fixed: gas is not estimated and an out-of-gas revert is not retried.
func WithFixedGasLimitAtLeast(limit uint64) Option {
	return func(o *TxOptions) {
		o.GasLimit = max(o.GasLimit, limit)
	}
}

func WithDefaultGasLimit(limit uint64) Option {
	return func(o *TxOptions) {
		if o.GasLimit == 0 {
			o.GasLimit = limit
		}
	}
}

func WithFeePriority(priority int) Option {
	return func(o *TxOptions) {
		o.Fee.Priority = priority
	}
}

func WithTipBoost(percent int) Option {
	return func(o *TxOptions) {
		o.Fee.TipBoostPercent = percent
	}
}

func Apply(opts ...Option) TxOptions {
	var o TxOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
