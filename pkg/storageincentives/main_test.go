// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	bmt.SetSIMDOptIn(true)
	goleak.VerifyTestMain(m)
}
