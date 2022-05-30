// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cidv1_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/resolver/cidv1"
)

func TestCIDResolver(t *testing.T) {
	r := cidv1.Resolver{}
	t.Cleanup(func() {
		err := r.Close()
		if err != nil {
			t.Fatal("failed closing")
		}
	})

	t.Run("resolve manifest CID", func(t *testing.T) {
		addr, err := r.Resolve("bah5acgzazjrvpieogf6rl3cwb7xtjzgel6hrt4a4g4vkody5u4v7u7y2im4a")
		if err != nil {
			t.Fatal(err)
		}

		expected := "ca6357a08e317d15ec560fef34e4c45f8f19f01c372aa70f1da72bfa7f1a4338"
		if addr.String() != expected {
			t.Fatalf("unexpected address resolved exp %s found %s", expected, addr.String())
		}
	})
	t.Run("resolve feed CID", func(t *testing.T) {
		addr, err := r.Resolve("bah5qcgzazjrvpieogf6rl3cwb7xtjzgel6hrt4a4g4vkody5u4v7u7y2im4a")
		if err != nil {
			t.Fatal(err)
		}

		expected := "ca6357a08e317d15ec560fef34e4c45f8f19f01c372aa70f1da72bfa7f1a4338"
		if addr.String() != expected {
			t.Fatalf("unexpected address resolved exp %s found %s", expected, addr.String())
		}
	})
	t.Run("fail other codecs", func(t *testing.T) {
		_, err := r.Resolve("bafybeiekkklkqtypmqav6ytqjbdqucxfwuk5cgige4245d2qhkccuyfnly")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
