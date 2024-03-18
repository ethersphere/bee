// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf_test

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("time.Sleep"),
		goleak.IgnoreTopFunction("io.(*pipe).read"),
		goleak.IgnoreTopFunction("io.(*pipe).write"),
		goleak.IgnoreTopFunction("github.com/ethersphere/bee/v2/pkg/p2p/protobuf_test.newMessageWriter.func1"),
	)
}
