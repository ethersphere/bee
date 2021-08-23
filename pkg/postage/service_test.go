// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	pstoremock "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
)

// TestSaveLoad tests the idempotence of saving and loading the postage.Service
// with all the active stamp issuers.
func TestSaveLoad(t *testing.T) {
	store := storemock.NewStateStore()
	pstore := pstoremock.New()
	saved := func(id int64) postage.Service {
		ps, err := postage.NewService(store, pstore, id)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 16; i++ {
			ps.Add(newTestStampIssuer(t, 1000))
		}
		if err := ps.Close(); err != nil {
			t.Fatal(err)
		}
		return ps
	}
	loaded := func(id int64) postage.Service {
		ps, err := postage.NewService(store, pstore, id)
		if err != nil {
			t.Fatal(err)
		}
		return ps
	}
	test := func(id int64) {
		psS := saved(id)
		psL := loaded(id)
		if !reflect.DeepEqual(psS.StampIssuers(), psL.StampIssuers()) {
			t.Fatalf("load(save(service)) != service\n%v\n%v", psS.StampIssuers(), psL.StampIssuers())
		}
	}
	test(0)
	test(1)
}

func TestGetStampIssuer(t *testing.T) {
	store := storemock.NewStateStore()
	testChainState := postagetesting.NewChainState()
	if testChainState.Block < uint64(postage.BlockThreshold) {
		testChainState.Block += uint64(postage.BlockThreshold + 1)
	}
	validBlockNumber := testChainState.Block - uint64(postage.BlockThreshold+1)
	pstore := pstoremock.New(pstoremock.WithChainState(testChainState))
	ps, err := postage.NewService(store, pstore, int64(0))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([][]byte, 8)
	for i := range ids {
		id := make([]byte, 32)
		_, err := io.ReadFull(crand.Reader, id)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if i == 0 {
			continue
		}

		var shift uint64 = 0
		if i > 3 {
			shift = uint64(i)
		}
		ps.Add(postage.NewStampIssuer(string(id), "", id, big.NewInt(3), 16, 8, validBlockNumber+shift, true))
	}
	t.Run("found", func(t *testing.T) {
		for _, id := range ids[1:4] {
			st, err := ps.GetStampIssuer(id)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if st.Label() != string(id) {
				t.Fatalf("wrong issuer returned")
			}
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, err := ps.GetStampIssuer(ids[0])
		if err != postage.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
	t.Run("not usable", func(t *testing.T) {
		for _, id := range ids[4:] {
			_, err := ps.GetStampIssuer(id)
			if err != postage.ErrNotUsable {
				t.Fatalf("expected ErrNotUsable, got %v", err)
			}
		}
	})
	t.Run("recovered", func(t *testing.T) {
		b := postagetesting.MustNewBatch()
		b.Start = validBlockNumber
		ps.HandleCreate(b)
		st, err := ps.GetStampIssuer(b.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if st.Label() != "recovered" {
			t.Fatal("wrong issuer returned")
		}
	})
	t.Run("topup", func(t *testing.T) {
		ps.HandleTopUp(ids[1], big.NewInt(10))
		_, err := ps.GetStampIssuer(ids[1])
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ps.StampIssuers()[0].Amount().Cmp(big.NewInt(10)) != 0 {
			t.Fatalf("expected amount %d got %d", 10, ps.StampIssuers()[0].Amount().Int64())
		}
	})
	t.Run("dilute", func(t *testing.T) {
		ps.HandleDepthIncrease(ids[2], 17, big.NewInt(1))
		_, err := ps.GetStampIssuer(ids[2])
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ps.StampIssuers()[1].Amount().Cmp(big.NewInt(1)) != 0 {
			t.Fatalf("expected amount %d got %d", 1, ps.StampIssuers()[1].Amount().Int64())
		}
		if ps.StampIssuers()[1].Depth() != 17 {
			t.Fatalf("expected depth %d got %d", 17, ps.StampIssuers()[1].Depth())
		}
	})
}
