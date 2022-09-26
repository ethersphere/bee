// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/api"
)

func TestToFileSizeBucket(t *testing.T) {
	t.Parallel()

	var want int64 = 300000
	bucket := api.ToFileSizeBucket(want)
	if bucket < want {
		t.Fatalf("bucket should be greater than filesize")
	}

	want = 5000000
	bucket = api.ToFileSizeBucket(want)
	if bucket != want {
		t.Fatalf("bucket should be exactly 5000000")
	}

	overBound := api.FileSizeBucketsKBytes[len(api.FileSizeBucketsKBytes)-1]*1000 + 1
	bucket = api.ToFileSizeBucket(overBound)
	if bucket != api.FileSizeBucketsKBytes[len(api.FileSizeBucketsKBytes)-1]*1000 {
		t.Fatalf("bucket should be the last bucket")
	}
}
