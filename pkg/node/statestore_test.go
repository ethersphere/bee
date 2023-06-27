// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestInitStamperStore(t *testing.T) {
	dataDir := t.TempDir()
	stateStore, _, err := InitStateStore(log.Noop, dataDir, 100_000)
	if err != nil {
		t.Fatal(err)
	}

	ids := make(map[string]int)

	// add 10 stamps to the state store
	for i := 0; i < 10; i++ {
		bID := make([]byte, 32)
		_, err = rand.Read(bID)
		if err != nil {
			t.Fatal(err)
		}
		si := postage.NewStampIssuer("", "", bID, big.NewInt(3), 11, 10, 1000, true)
		err = stateStore.Put(fmt.Sprintf("postage%s", string(si.ID())), si)
		if err != nil {
			t.Fatal(err)
		}
		ids[string(si.ID())] = 0
	}

	stamperStore, err := InitStamperStore(log.Noop, dataDir, stateStore)
	if err != nil {
		t.Fatal("init stamper store should migrate stamps from state store", err)
	}

	err = stamperStore.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(postage.StampIssuerItem) },
		}, func(result storage.Result) (bool, error) {
			issuer := result.Entry.(*postage.StampIssuerItem).Issuer
			ids[string(issuer.ID())]++
			return false, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	var got int
	for _, v := range ids {
		if v > 0 {
			got++
		}
	}
	if got != 10 {
		t.Fatalf("want %d stamps. got %d", 10, got)
	}

	t.Cleanup(func() {
		err = stateStore.Close()
		if err != nil {
			t.Fatal(err)
		}
		err = stamperStore.Close()
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second)
	})
}
