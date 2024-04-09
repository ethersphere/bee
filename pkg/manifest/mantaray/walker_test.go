// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/manifest/mantaray"
)

func TestWalkNode(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		toAdd    [][]byte
		expected [][]byte
	}{
		{
			name: "simple",
			toAdd: [][]byte{
				[]byte("index.html"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("robots.txt"),
			},
			expected: [][]byte{
				[]byte(""),
				[]byte("i"),
				[]byte("img/"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("index.html"),
				[]byte("robots.txt"),
			},
		},
	} {
		ctx := context.Background()
		tc := tc

		createTree := func(t *testing.T, toAdd [][]byte) *mantaray.Node {
			t.Helper()

			n := mantaray.New()

			for i := 0; i < len(toAdd); i++ {
				c := toAdd[i]
				e := append(make([]byte, 32-len(c)), c...)
				err := n.Add(ctx, c, e, nil, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
			return n
		}

		pathExistsInRightSequence := func(found []byte, expected [][]byte, walkedCount int) bool {
			rightPathInSequence := false

			c := expected[walkedCount]
			if bytes.Equal(found, c) {
				rightPathInSequence = true
			}

			return rightPathInSequence
		}

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			n := createTree(t, tc.toAdd)

			walkedCount := 0

			walker := func(path []byte, node *mantaray.Node, err error) error {

				if !pathExistsInRightSequence(path, tc.expected, walkedCount) {
					return fmt.Errorf("walkFn returned unexpected path: %s", path)
				}
				walkedCount++
				return nil
			}

			// Expect no errors.
			err := n.WalkNode(ctx, []byte{}, nil, walker)
			if err != nil {
				t.Fatalf("no error expected, found: %s", err)
			}

			if len(tc.expected) != walkedCount {
				t.Errorf("expected %d nodes, got %d", len(tc.expected), walkedCount)
			}
		})

		t.Run(tc.name+"/with load save", func(t *testing.T) {
			t.Parallel()

			n := createTree(t, tc.toAdd)

			ls := newMockLoadSaver()

			err := n.Save(ctx, ls)
			if err != nil {
				t.Fatal(err)
			}

			n2 := mantaray.NewNodeRef(n.Reference())

			walkedCount := 0

			walker := func(path []byte, node *mantaray.Node, err error) error {

				if !pathExistsInRightSequence(path, tc.expected, walkedCount) {
					return fmt.Errorf("walkFn returned unexpected path: %s", path)
				}
				walkedCount++
				return nil
			}

			// Expect no errors.
			err = n2.WalkNode(ctx, []byte{}, ls, walker)
			if err != nil {
				t.Fatalf("no error expected, found: %s", err)
			}

			if len(tc.expected) != walkedCount {
				t.Errorf("expected %d nodes, got %d", len(tc.expected), walkedCount)
			}
		})
	}
}
