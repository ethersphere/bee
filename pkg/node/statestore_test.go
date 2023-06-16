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
	stateStore, err := InitStateStore(log.Noop, dataDir)
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

	// init stamper store should migrate 10 stamps from state store
	stamperStore, err := InitStamperStore(log.Noop, dataDir, stateStore)
	if err != nil {
		t.Fatal(err)
	}

	// check stamper store has 10 stamps
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

	nMissing := 0
	for _, v := range ids {
		if v != 1 {
			nMissing++
		}
	}
	if nMissing > 0 {
		t.Fatalf("missing %d stamps", nMissing)
	}

	t.Cleanup(func() {
		_ = stateStore.Close()
		_ = stamperStore.Close()
		time.Sleep(1 * time.Second)
	})
}
