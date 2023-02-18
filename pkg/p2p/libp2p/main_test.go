// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p_test

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
		goleak.IgnoreTopFunction("sync.runtime_Semacquire"),
		goleak.IgnoreTopFunction("net.ParseCIDR"),
		goleak.IgnoreTopFunction("github.com/ipfs/go-log/writer.(*MirrorWriter).logRoutine"),
		goleak.IgnoreTopFunction("github.com/huin/goupnp/httpu.(*MultiClient).Do.func2"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-flow-metrics.(*sweeper).run"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.(*prefixTrie).insert"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.newPathprefixTrie"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NetworkNumber.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NewNetwork"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.Network.LeastCommonBitPosition"),
	)
}
