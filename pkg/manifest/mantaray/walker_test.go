// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray_test

import (
	"bytes"
	"context"
	"fmt"
	"slices"
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
				[]byte("index.html.backup"),
				[]byte("index.html"),
				[]byte("img/test/oho.png"),
				[]byte("img/test/old/test.png.backup"),
				[]byte("img/test/old/test.png"),
				[]byte("img/2.png"),
				[]byte("img/1.png"),
				[]byte("robots.txt"),
			},
			expected: [][]byte{
				[]byte(""),
				[]byte("i"),
				[]byte("img/"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("img/test/o"),
				[]byte("img/test/oho.png"),
				[]byte("img/test/old/test.png"),
				[]byte("img/test/old/test.png.backup"),
				[]byte("index.html"),
				[]byte("index.html.backup"),
				[]byte("robots.txt"),
			},
		},
	} {
		ctx := context.Background()
		createTree := func(t *testing.T, toAdd [][]byte) *mantaray.Node {
			t.Helper()

			n := mantaray.New()

			for i := range toAdd {
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

// walkTestPaths are the paths of the trie TestWalkNode builds, and
// walkTestNodes the node paths a full walk of it visits, in order.
var (
	walkTestPaths = []string{
		"index.html.backup",
		"index.html",
		"img/test/oho.png",
		"img/test/old/test.png.backup",
		"img/test/old/test.png",
		"img/2.png",
		"img/1.png",
		"robots.txt",
	}
	walkTestNodes = []string{
		"",
		"i",
		"img/",
		"img/1.png",
		"img/2.png",
		"img/test/o",
		"img/test/oho.png",
		"img/test/old/test.png",
		"img/test/old/test.png.backup",
		"index.html",
		"index.html.backup",
		"robots.txt",
	}
)

// countingLoader counts the nodes a walk loads.
type countingLoader struct {
	mantaray.Loader
	loads int
}

func (c *countingLoader) Load(ctx context.Context, ref []byte) ([]byte, error) {
	c.loads++
	return c.Loader.Load(ctx, ref)
}

// saveWalkTestTrie saves the walk test trie and returns a loader for it and an
// unloaded reference to its root.
func saveWalkTestTrie(t *testing.T) (*countingLoader, *mantaray.Node) {
	t.Helper()
	ctx := context.Background()

	n := mantaray.New()
	for _, p := range walkTestPaths {
		e := append(make([]byte, 32-len(p)), p...)
		if err := n.Add(ctx, []byte(p), e, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	ls := newMockLoadSaver()
	if err := n.Save(ctx, ls); err != nil {
		t.Fatal(err)
	}
	return &countingLoader{Loader: ls}, mantaray.NewNodeRef(n.Reference())
}

func TestWalkNodeFrom(t *testing.T) {
	t.Parallel()

	starts := make([]string, 0, 2*len(walkTestNodes)+5)
	for _, p := range walkTestNodes {
		starts = append(starts, p, p+"\x00")
	}
	starts = append(starts, "a", "img/0", "img/test/old/", "j", "zzz")

	for _, start := range starts {
		t.Run(fmt.Sprintf("%q", start), func(t *testing.T) {
			t.Parallel()

			var want []string
			for _, p := range walkTestNodes {
				if p >= start {
					want = append(want, p)
				}
			}

			l, root := saveWalkTestTrie(t)
			var got []string
			err := root.WalkNodeFrom(context.Background(), nil, []byte(start), l, func(path []byte, _ *mantaray.Node, err error) error {
				if err != nil {
					return err
				}
				got = append(got, string(path))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestWalkNodeFromSkipsLoads(t *testing.T) {
	t.Parallel()

	walk := func(t *testing.T, start string) int {
		t.Helper()
		l, root := saveWalkTestTrie(t)
		err := root.WalkNodeFrom(context.Background(), nil, []byte(start), l, func([]byte, *mantaray.Node, error) error {
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return l.loads
	}

	if got := walk(t, ""); got != len(walkTestNodes) {
		t.Fatalf("full walk: got %d loads, want %d", got, len(walkTestNodes))
	}
	// only the root and "robots.txt": the "i" fork sorts wholly before start
	if got := walk(t, "robots.txt"); got != 2 {
		t.Fatalf("walk from robots.txt: got %d loads, want 2", got)
	}
	// everything but "img/1.png" and "img/2.png"
	if got := walk(t, "img/test/oho.png"); got != len(walkTestNodes)-2 {
		t.Fatalf("walk from img/test/oho.png: got %d loads, want %d", got, len(walkTestNodes)-2)
	}
}

func TestWalkNodeSkipSubtree(t *testing.T) {
	t.Parallel()

	l, root := saveWalkTestTrie(t)
	var got []string
	err := root.WalkNode(context.Background(), nil, l, func(path []byte, _ *mantaray.Node, err error) error {
		if err != nil {
			return err
		}
		got = append(got, string(path))
		if string(path) == "img/" {
			return mantaray.ErrSkipSubtree
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "i", "img/", "index.html", "index.html.backup", "robots.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	// the skipped subtree is not loaded
	if want := len(walkTestNodes) - 6; l.loads != want {
		t.Fatalf("got %d loads, want %d", l.loads, want)
	}
}
