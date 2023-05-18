// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(
		m,
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*nonrecursiveTree).dispatch"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*nonrecursiveTree).internal"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify.(*recursiveTree).dispatch"),
		goleak.IgnoreTopFunction("github.com/rjeczalik/notify._Cfunc_CFRunLoopRun"),
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*DB).compactionError"),
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*DB).tCompaction"),
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*DB).mCompaction"),
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*session).refLoop"),
		goleak.IgnoreTopFunction("github.com/syndtr/goleveldb/leveldb.(*DB).mpoolDrain"),
	)
}
