// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

func TestWalkNode(t *testing.T) {
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
				[]byte("index.html"),
				[]byte("img/"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("robots.txt"),
			},
		},
	} {
		ctx := context.Background()
		t.Run(tc.name, func(t *testing.T) {
			n := New()

			for i := 0; i < len(tc.toAdd); i++ {
				c := tc.toAdd[i]
				e := append(make([]byte, 32-len(c)), c...)
				err := n.Add(ctx, c, e, nil, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}

			walkedCount := 0

			walker := func(path []byte, node *Node, err error) error {
				walkedCount++

				pathFound := false

				for i := 0; i < len(tc.expected); i++ {
					c := tc.expected[i]
					if bytes.Equal(path, c) {
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
			err := n.WalkNode(ctx, []byte{}, nil, walker)
			if err != nil {
				t.Fatalf("no error expected, found: %s", err)
			}

			if len(tc.expected) != walkedCount {
				t.Errorf("expected %d nodes, got %d", len(tc.expected), walkedCount)
			}

		})
	}
}

func TestWalk(t *testing.T) {
	for _, tc := range []struct {
		name     string
		toAdd    [][]byte
		expected [][]byte
	}{
		{
			name: "simple",
			toAdd: [][]byte{
				[]byte("index.html"),
				[]byte("img/test/"),
				[]byte("img/test/oho.png"),
				[]byte("img/test/old/test.png"),
				[]byte("robots.txt"),
			},
			expected: [][]byte{
				[]byte("index.html"),
				[]byte("img"),
				[]byte("img/test"),
				[]byte("img/test/oho.png"),
				[]byte("img/test/old"),
				[]byte("img/test/old/test.png"),
				[]byte("robots.txt"),
			},
		},
	} {
		ctx := context.Background()
		t.Run(tc.name, func(t *testing.T) {
			n := New()

			for i := 0; i < len(tc.toAdd); i++ {
				c := tc.toAdd[i]
				e := append(make([]byte, 32-len(c)), c...)
				err := n.Add(ctx, c, e, nil, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}

			walkedCount := 0

			walker := func(path []byte, isDir bool, err error) error {
				walkedCount++

				pathFound := false

				for i := 0; i < len(tc.expected); i++ {
					c := tc.expected[i]
					if bytes.Equal(path, c) {
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
			err := n.Walk(ctx, []byte{}, nil, walker)
			if err != nil {
				t.Fatalf("no error expected, found: %s", err)
			}

			if len(tc.expected) != walkedCount {
				t.Errorf("expected %d nodes, got %d", len(tc.expected), walkedCount)
			}

		})
	}
}
