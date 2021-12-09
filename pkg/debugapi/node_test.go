// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
)

func TestBeeNodeMode_String(t *testing.T) {
	const nonExistingMode debugapi.BeeNodeMode = 4

	mapping := map[string]string{
		debugapi.LightMode.String(): "light",
		debugapi.FullMode.String():  "full",
		debugapi.DevMode.String():   "dev",
		nonExistingMode.String():    "unknown",
	}

	for have, want := range mapping {
		if have != want {
			t.Fatalf("unexpected bee node mode: have %q; want %q", have, want)
		}
	}
}
