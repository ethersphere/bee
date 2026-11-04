// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accesscontrol_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	kvsmock "github.com/ethersphere/bee/v2/pkg/accesscontrol/kvs/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestDecryptRefRejectsMalformedAccessKey checks that an ACT slot decrypting
// to an empty access key returns an error instead of reaching
// encryption.transform, whose loop advances by the key length.
func TestDecryptRefRejectsMalformedAccessKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	key := getPrivKey(0)
	session := accesscontrol.NewDefaultSession(key)
	al := accesscontrol.NewLogic(session)

	// A distinct store: kvsmock.New() shares one global map between instances.
	act := kvsmock.NewReference(swarm.RandAddress(t))
	if err := al.AddGrantee(ctx, act, &key.PublicKey, &key.PublicKey); err != nil {
		t.Fatal(err)
	}

	// Overwrite the publisher's slot with an empty value.
	keys, err := session.Key(&key.PublicKey, [][]byte{{0}, {1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := act.Put(ctx, keys[0], []byte{}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := al.DecryptRef(ctx, act, swarm.RandAddress(t), &key.PublicKey)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, accesscontrol.ErrInvalidAccessKey) {
			t.Fatalf("got error %v, want %v", err, accesscontrol.ErrInvalidAccessKey)
		}
	case <-time.After(5 * time.Second):
		// The goroutine cannot be cancelled; it is reaped when the binary exits.
		t.Fatal("DecryptRef did not return: an empty access key reached encryption.transform")
	}
}
