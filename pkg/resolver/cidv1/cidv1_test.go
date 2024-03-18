// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cidv1_test

import (
	"errors"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/resolver"
	"github.com/ethersphere/bee/v2/pkg/resolver/cidv1"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestCIDResolver(t *testing.T) {
	t.Parallel()

	r := cidv1.Resolver{}
	testutil.CleanupCloser(t, r)

	t.Run("resolve manifest CID", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

		_, err := r.Resolve("bafybeiekkklkqtypmqav6ytqjbdqucxfwuk5cgige4245d2qhkccuyfnly")
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("fail on invalid CID", func(t *testing.T) {
		t.Parallel()

		_, err := r.Resolve("bafybeiekk")
		if !errors.Is(err, resolver.ErrParse) {
			t.Fatal("expected error", resolver.ErrParse, "got", err)
		}
	})
}
