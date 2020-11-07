// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"io"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	storemock "github.com/ethersphere/bee/pkg/statestore/mock"
)

// TestSaveLoad tests the idempotence of saving and loading the postage.Service
// with all the active stamp issuers.
func TestSaveLoad(t *testing.T) {
	store := storemock.NewStateStore()
	saved := func(id int64) *postage.Service {
		ps := postage.NewService(store, id)
		for i := 0; i < 16; i++ {
			ps.Add(newTestStampIssuer(t))
		}
		if err := ps.Save(); err != nil {
			t.Fatal(err)
		}
		return ps
	}
	loaded := func(id int64) *postage.Service {
		ps := postage.NewService(store, id)
		if err := ps.Load(); err != nil {
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
	ps := postage.NewService(store, int64(0))
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
		ps.Add(postage.NewStampIssuer(string(id), "", id, 16, 8))
	}
	t.Run("found", func(t *testing.T) {
		for _, id := range ids[1:] {
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
}
