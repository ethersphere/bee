// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"testing"
)

func TestBeeNodeMode_String(t *testing.T) {
	const nonExistingMode BeeNodeMode = 4

	mapping := map[string]string{
		LightMode.String():       "light",
		FullMode.String():        "full",
		DevMode.String():         "dev",
		nonExistingMode.String(): "unknown",
	}

	for have, want := range mapping {
		if have != want {
			t.Fatalf("got unexpected bee node mode status string %q; wanted %q", have, want)
		}
	}
}
