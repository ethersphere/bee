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
		goleak.IgnoreTopFunction("github.com/libp2p/go-flow-metrics.(*sweeper).runActive"),
		goleak.IgnoreTopFunction("github.com/huin/goupnp/httpu.(*MultiClient).Do.func2"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-flow-metrics.(*sweeper).run"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.(*prefixTrie).insert"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.newPathprefixTrie"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NetworkNumber.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NewNetwork"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.Network.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-libp2p/p2p/transport/quicreuse.(*reuse).gc"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-libp2p/p2p/host/resource-manager.(*resourceManager).background"),
		goleak.IgnoreTopFunction("github.com/quic-go/quic-go.(*packetHandlerMap).runCloseQueue"),
		goleak.IgnoreTopFunction("github.com/quic-go/quic-go.(*Transport).runSendQueue"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).roundTrip"),
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("net/http.(*persistConn).writeLoop "),
		goleak.IgnoreTopFunction("net.(*netFD).connect.func2"),
		goleak.IgnoreTopFunction("net/http.(*Transport).getConn"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.(*prefixTrie).insert"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.newPathprefixTrie"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NetworkNumber.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NewNetwork"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.Network.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-flow-metrics.(*sweeper).runActive"),
	)
}
