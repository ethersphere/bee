// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"
)

type nodeEntry struct {
	path     []byte
	entry    []byte
	metadata map[string]string
}

func TestNilPath(t *testing.T) {
	ctx := context.Background()
	n := New()
	_, err := n.Lookup(ctx, nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestAddAndLookup(t *testing.T) {
	ctx := context.Background()
	n := New()
	testCases := [][]byte{
		[]byte("aaaaaa"),
		[]byte("aaaaab"),
		[]byte("abbbb"),
		[]byte("abbba"),
		[]byte("bbbbba"),
		[]byte("bbbaaa"),
		[]byte("bbbaab"),
		[]byte("aa"),
		[]byte("b"),
	}
	for i := 0; i < len(testCases); i++ {
		c := testCases[i]
		e := append(make([]byte, 32-len(c)), c...)
		err := n.Add(ctx, c, e, nil, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		for j := 0; j < i; j++ {
			d := testCases[j]
			m, err := n.Lookup(ctx, d, nil)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			de := append(make([]byte, 32-len(d)), d...)
			if !bytes.Equal(m, de) {
				t.Fatalf("expected value %x, got %x", d, m)
			}
		}
	}
}

func TestAddAndLookupNode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		toAdd [][]byte
	}{
		{
			name: "a",
			toAdd: [][]byte{
				[]byte("aaaaaa"),
				[]byte("aaaaab"),
				[]byte("abbbb"),
				[]byte("abbba"),
				[]byte("bbbbba"),
				[]byte("bbbaaa"),
				[]byte("bbbaab"),
				[]byte("aa"),
				[]byte("b"),
			},
		},
		{
			name: "simple",
			toAdd: [][]byte{
				[]byte("/"),
				[]byte("index.html"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("robots.txt"),
			},
		},
		{
			// mantaray.nodePrefixMaxSize number of '.'
			name: "nested-value-node-is-recognized",
			toAdd: [][]byte{
				[]byte("..............................@"),
				[]byte(".............................."),
			},
		},
		{
			name: "nested-prefix-is-not-collapsed",
			toAdd: [][]byte{
				[]byte("index.html"),
				[]byte("img/1.png"),
				[]byte("img/2/test1.png"),
				[]byte("img/2/test2.png"),
				[]byte("robots.txt"),
			},
		},
		{
			name: "conflicting-path",
			toAdd: [][]byte{
				[]byte("app.js.map"),
				[]byte("app.js"),
			},
		},
		{
			name: "spa-website",
			toAdd: [][]byte{
				[]byte("css/"),
				[]byte("css/app.css"),
				[]byte("favicon.ico"),
				[]byte("img/"),
				[]byte("img/logo.png"),
				[]byte("index.html"),
				[]byte("js/"),
				[]byte("js/chunk-vendors.js.map"),
				[]byte("js/chunk-vendors.js"),
				[]byte("js/app.js.map"),
				[]byte("js/app.js"),
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
				for j := 0; j < i+1; j++ {
					d := tc.toAdd[j]
					node, err := n.LookupNode(ctx, d, nil)
					if err != nil {
						t.Fatalf("expected no error, got %v", err)
					}
					if !node.IsValueType() {
						t.Fatalf("expected value type, got %v", strconv.FormatInt(int64(node.nodeType), 2))
					}
					de := append(make([]byte, 32-len(d)), d...)
					if !bytes.Equal(node.entry, de) {
						t.Fatalf("expected value %x, got %x", d, node.entry)
					}
				}
			}
		})
	}
}

func TestRemove(t *testing.T) {
	for _, tc := range []struct {
		name     string
		toAdd    []nodeEntry
		toRemove [][]byte
	}{
		{
			name: "simple",
			toAdd: []nodeEntry{
				{
					path: []byte("/"),
					metadata: map[string]string{
						"index-document": "index.html",
					},
				},
				{
					path: []byte("index.html"),
				},
				{
					path: []byte("img/1.png"),
				},
				{
					path: []byte("img/2.png"),
				},
				{
					path: []byte("robots.txt"),
				},
			},
			toRemove: [][]byte{
				[]byte("img/2.png"),
			},
		},
		{
			name: "nested-prefix-is-not-collapsed",
			toAdd: []nodeEntry{
				{
					path: []byte("index.html"),
				},
				{
					path: []byte("img/1.png"),
				},
				{
					path: []byte("img/2/test1.png"),
				},
				{
					path: []byte("img/2/test2.png"),
				},
				{
					path: []byte("robots.txt"),
				},
			},
			toRemove: [][]byte{
				[]byte("img/2/test1.png"),
			},
		},
	} {
		ctx := context.Background()
		t.Run(tc.name, func(t *testing.T) {
			n := New()

			for i := 0; i < len(tc.toAdd); i++ {
				c := tc.toAdd[i].path
				e := tc.toAdd[i].entry
				if len(e) == 0 {
					e = append(make([]byte, 32-len(c)), c...)
				}
				m := tc.toAdd[i].metadata
				err := n.Add(ctx, c, e, m, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				for j := 0; j < i; j++ {
					d := tc.toAdd[j].path
					m, err := n.Lookup(ctx, d, nil)
					if err != nil {
						t.Fatalf("expected no error, got %v", err)
					}
					de := append(make([]byte, 32-len(d)), d...)
					if !bytes.Equal(m, de) {
						t.Fatalf("expected value %x, got %x", d, m)
					}
				}
			}

			for i := 0; i < len(tc.toRemove); i++ {
				c := tc.toRemove[i]
				err := n.Remove(ctx, c, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				_, err = n.Lookup(ctx, c, nil)
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("expected not found error, got %v", err)
				}
			}

		})
	}
}

func TestHasPrefix(t *testing.T) {
	for _, tc := range []struct {
		name        string
		toAdd       [][]byte
		testPrefix  [][]byte
		shouldExist []bool
	}{
		{
			name: "simple",
			toAdd: [][]byte{
				[]byte("index.html"),
				[]byte("img/1.png"),
				[]byte("img/2.png"),
				[]byte("robots.txt"),
			},
			testPrefix: [][]byte{
				[]byte("img/"),
				[]byte("images/"),
			},
			shouldExist: []bool{
				true,
				false,
			},
		},
		{
			name: "nested-single",
			toAdd: [][]byte{
				[]byte("some-path/file.ext"),
			},
			testPrefix: [][]byte{
				[]byte("some-path/"),
				[]byte("some-path/file"),
				[]byte("some-other-path/"),
			},
			shouldExist: []bool{
				true,
				true,
				false,
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

			for i := 0; i < len(tc.testPrefix); i++ {
				testPrefix := tc.testPrefix[i]
				shouldExist := tc.shouldExist[i]

				exists, err := n.HasPrefix(ctx, testPrefix, nil)
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}

				if shouldExist != exists {
					t.Errorf("expected prefix path %s to be %t, was %t", testPrefix, shouldExist, exists)
				}
			}

		})
	}
}
