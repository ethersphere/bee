// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/eip712"
)

type signerMock struct {
	signTx        func(transaction *types.Transaction) (*types.Transaction, error)
	signTypedData func(*eip712.TypedData) ([]byte, error)
}

func (*signerMock) EthereumAddress() (common.Address, error) {
	return common.Address{}, nil
}

func (*signerMock) Sign(data []byte) ([]byte, error) {
	return nil, nil
}

func (m *signerMock) SignTx(transaction *types.Transaction) (*types.Transaction, error) {
	return m.signTx(transaction)
}

func (*signerMock) PublicKey() (*ecdsa.PublicKey, error) {
	return nil, nil
}

func (m *signerMock) SignTypedData(d *eip712.TypedData) ([]byte, error) {
	return m.signTypedData(d)
}

func New(opts ...Option) crypto.Signer {
	mock := new(signerMock)
	for _, o := range opts {
		o.apply(mock)
	}
	return mock
}

// Option is the option passed to the mock Chequebook service
type Option interface {
	apply(*signerMock)
}

type optionFunc func(*signerMock)

func (f optionFunc) apply(r *signerMock) { f(r) }

func WithSignTxFunc(f func(transaction *types.Transaction) (*types.Transaction, error)) Option {
	return optionFunc(func(s *signerMock) {
		s.signTx = f
	})
}

func WithSignTypedDataFunc(f func(*eip712.TypedData) ([]byte, error)) Option {
	return optionFunc(func(s *signerMock) {
		s.signTypedData = f
	})
}
