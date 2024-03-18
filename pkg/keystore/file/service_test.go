// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/keystore/file"
	"github.com/ethersphere/bee/v2/pkg/keystore/test"
)

func TestService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	test.Service(t, file.New(dir))
}
