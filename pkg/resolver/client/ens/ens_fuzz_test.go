// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ens_test

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/resolver/client/ens"
)

func FuzzDNSEncode(f *testing.F) {
	for _, seed := range []string{
		"eth",
		"swarm.eth",
		"sub.solarpunkid.eth",
		"",
		".",
		".eth",
		"swarm.",
		"swarm..eth",
		strings.Repeat("a", 253),
		strings.Repeat("a", 255) + ".eth",
		strings.Repeat("a", 256) + ".eth",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, name string) {
		buf, err := ens.DNSEncode(name)
		if err != nil {
			return
		}

		if len(buf) > 255 {
			t.Fatalf("encoded length %d exceeds 255", len(buf))
		}
		if len(buf) != len(name)+2 {
			t.Fatalf("encoded length: got %d, want %d", len(buf), len(name)+2)
		}

		// Decoding the wire format must give back the original name.
		var labels []string
		for i := 0; ; {
			n := int(buf[i])
			if n == 0 {
				if i != len(buf)-1 {
					t.Fatalf("terminator at %d, want %d", i, len(buf)-1)
				}
				break
			}
			if i+1+n >= len(buf) {
				t.Fatalf("label at %d overflows buffer", i)
			}
			labels = append(labels, string(buf[i+1:i+1+n]))
			i += 1 + n
		}
		if got := strings.Join(labels, "."); got != name {
			t.Fatalf("decoded name: got %q, want %q", got, name)
		}
	})
}
