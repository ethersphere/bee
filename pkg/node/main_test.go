// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*nonrecursiveTree).dispatch"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*recursiveTree).dispatch"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*nonrecursiveTree).internal"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify._Cfunc_CFRunLoopRun"),
		goleak.IgnoreTopFunction("github.com/ipfs/go-log/writer.(*MirrorWriter).logRoutine"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NetworkNumber.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.NewNetwork"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger/net.Network.LeastCommonBitPosition"),
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreTopFunction("github.com/libp2p/go-cidranger.(*prefixTrie).insert"),
	)
}
