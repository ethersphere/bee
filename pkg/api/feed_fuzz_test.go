// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/crypto"
)

// FuzzFeedPostHandler drives arbitrary HTTP feed creation requests through the feed
// router path, testing the control-flow bug (CF-01) where error handling in feed creation
// must never fall through to dereference a nil manifest.
func FuzzFeedPostHandler(f *testing.F) {
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		f.Fatal(err)
	}
	owner, err := crypto.NewEthereumAddress(privKey.PublicKey)
	if err != nil {
		f.Fatal(err)
	}
	ownerHex := hex.EncodeToString(owner)

	f.Add(ownerHex, "0000000000000000000000000000000000000000000000000000000000000000")
	f.Add(ownerHex, "topic")
	f.Add("invalid-owner", "topic")
	f.Add("", "")

	f.Fuzz(func(t *testing.T, ownerStr, topicStr string) {
		client, _, _, _ := newTestServer(t, testServerOptions{})

		url := fmt.Sprintf("/feeds/%s/%s", ownerStr, topicStr)
		// Request through test server; must never panic
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			_ = resp.Body.Close()
		}
	})
}
