// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		// pkg/p2p package has some leak issues, we ignore them here as they are not in current scope
		goleak.IgnoreTopFunction("github.com/ethersphere/bee/v2/pkg/p2p/protobuf.Reader.ReadMsgWithContext"),
		goleak.IgnoreTopFunction("github.com/ethersphere/bee/v2/pkg/p2p/streamtest.(*record).Read"),
	)
}
