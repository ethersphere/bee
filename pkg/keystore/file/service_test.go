// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/keystore/file"
	"github.com/ethersphere/bee/pkg/keystore/test"
)

func TestService(t *testing.T) {
	dir, err := ioutil.TempDir("", "bzz-keystore-file-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	test.Service(t, file.New(dir))
}
