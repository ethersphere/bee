// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package simple_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/manifest/simple"
)

func TestWalkEntry(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := simple.NewManifest()

			// add entries
			for _, e := range tc.entries {
				err := m.Add(e.path, e.reference, e.metadata)
				if err != nil {
					t.Fatal(err)
				}
			}

			manifestLen := m.Length()

			if len(tc.entries) != manifestLen {
				t.Fatalf("expected %d entries, found %d", len(tc.entries), manifestLen)
			}

			walkedCount := 0

			walker := func(path string, entry simple.Entry, err error) error {
				walkedCount++

				pathFound := false

				for i := 0; i < len(tc.entries); i++ {
					p := tc.entries[i].path
					if path == p {
						pathFound = true
						break
					}
				}

				if !pathFound {
					return fmt.Errorf("walkFn returned unknown path: %s", path)
				}

				return nil
			}
			// Expect no errors.
			err := m.WalkEntry("", walker)
			if err != nil {
				t.Fatalf("no error expected, found: %s", err)
			}

			if len(tc.entries) != walkedCount {
				t.Errorf("expected %d nodes, got %d", len(tc.entries), walkedCount)
			}
		})
	}
}
