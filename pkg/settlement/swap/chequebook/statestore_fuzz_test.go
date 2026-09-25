// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook_test

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/chequebook"
	"github.com/ethersphere/bee/v2/pkg/storage"
	transactionmock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

type rawStore struct {
	data map[string][]byte
}

func newRawStore() *rawStore {
	return &rawStore{data: make(map[string][]byte)}
}

func (r *rawStore) Get(key string, i any) error {
	b, ok := r.data[key]
	if !ok {
		return storage.ErrNotFound
	}
	return json.Unmarshal(b, i)
}

func (r *rawStore) Put(key string, i any) error {
	return nil
}

func (r *rawStore) Delete(key string) error {
	delete(r.data, key)
	return nil
}

func (r *rawStore) Iterate(prefix string, iterFunc storage.StateIterFunc) error {
	return nil
}

func (r *rawStore) Close() error {
	return nil
}

func (r *rawStore) setRaw(key string, b []byte) {
	r.data[key] = b
}

// FuzzChequeStoreStatestoreCorruption tests that ChequeStore (LastCheque, ReceiveCheque)
// and Service.LastCheque tolerate arbitrary corrupted or literal null JSON statestore entries
// without nil-dereferencing or panicking (NIL-02, NIL-03, NIL-04).
func FuzzChequeStoreStatestoreCorruption(f *testing.F) {
	// Seed 1: JSON null literal
	f.Add([]byte("null"))
	// Seed 2: Empty JSON object
	f.Add([]byte("{}"))
	// Seed 3: JSON object with null fields
	f.Add([]byte(`{"Cheque":{"beneficiary":"0x0000000000000000000000000000000000000000","recipient":"0x0000000000000000000000000000000000000000","cumulativePayout":null}}`))
	// Seed 4: Random string / non-json
	f.Add([]byte("invalid-raw-bytes"))

	f.Fuzz(func(t *testing.T, corruptedData []byte) {
		store := newRawStore()
		beneficiary := common.HexToAddress("0xbe")
		chequebookAddr := common.HexToAddress("0xcb")

		// Corrupt the last-received-cheque key in statestore with raw bytes
		store.setRaw(chequebook.LastReceivedChequeKey(chequebookAddr), corruptedData)
		// Corrupt the last-issued-cheque key
		store.setRaw(chequebook.LastIssuedChequeKey(chequebookAddr), corruptedData)

		cs := chequebook.NewChequeStore(
			store,
			&factoryMock{verifyChequebook: func(context.Context, common.Address) error { return nil }},
			1,
			beneficiary,
			transactionmock.New(),
			func(*chequebook.SignedCheque, int64) (common.Address, error) { return common.Address{}, nil },
		)

		// 1. LastCheque must never panic
		lastCheque, err := cs.LastCheque(chequebookAddr)
		if err == nil && lastCheque == nil {
			t.Fatal("NIL-02: LastCheque returned (nil, nil)")
		}

		// 2. ReceiveCheque must never panic when reading corrupted last-received cheque
		incomingCheque := &chequebook.SignedCheque{
			Cheque: chequebook.Cheque{
				Beneficiary:      beneficiary,
				Chequebook:       chequebookAddr,
				CumulativePayout: big.NewInt(100),
			},
			Signature: make([]byte, 65),
		}
		_, _ = cs.ReceiveCheque(context.Background(), incomingCheque, big.NewInt(1), big.NewInt(0))

		// 3. Service.LastCheque must never panic
		svc, err := chequebook.New(nil, common.Address{}, common.Address{}, store, nil, nil)
		if err == nil {
			issuedCheque, err := svc.LastCheque(beneficiary)
			if err == nil && issuedCheque == nil {
				t.Fatal("NIL-04: svc.LastCheque returned (nil, nil)")
			}
		}
	})
}
