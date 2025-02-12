// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pseudorand_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/util/testutil/pseudorand"
)

func TestReader(t *testing.T) {
	size := 42000
	seed := make([]byte, 32)
	r := pseudorand.NewReader(seed, size)
	content, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("deterministicity", func(t *testing.T) {
		r2 := pseudorand.NewReader(seed, size)
		content2, err := io.ReadAll(r2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(content, content2) {
			t.Fatal("content mismatch")
		}
	})
	t.Run("randomness", func(t *testing.T) {
		bufSize := 4096
		if bytes.Equal(content[:bufSize], content[bufSize:2*bufSize]) {
			t.Fatal("buffers should not match")
		}
	})
	t.Run("re-readability", func(t *testing.T) {
		ns, err := r.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal(err)
		}
		if ns != 0 {
			t.Fatal("seek mismatch")
		}
		var read []byte
		buf := make([]byte, 8200)
		total := 0
		for {
			s := rand.Intn(820)
			n, err := r.Read(buf[:s])
			total += n
			read = append(read, buf[:n]...)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		read = read[:total]
		if len(read) != len(content) {
			t.Fatal("content length mismatch. expected", len(content), "got", len(read))
		}
		if !bytes.Equal(content, read) {
			t.Fatal("content mismatch")
		}
	})
	t.Run("comparison", func(t *testing.T) {
		ns, err := r.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal(err)
		}
		if ns != 0 {
			t.Fatal("seek mismatch")
		}
		if eq, err := r.Equal(bytes.NewBuffer(content)); err != nil {
			t.Fatal(err)
		} else if !eq {
			t.Fatal("content mismatch")
		}
		ns, err = r.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal(err)
		}
		if ns != 0 {
			t.Fatal("seek mismatch")
		}
		if eq, err := r.Equal(bytes.NewBuffer(content[:len(content)-1])); err != nil {
			t.Fatal(err)
		} else if eq {
			t.Fatal("content should match")
		}
	})
	t.Run("seek and match", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			off := rand.Intn(size)
			n := rand.Intn(size - off)
			t.Run(fmt.Sprintf("off=%d n=%d", off, n), func(t *testing.T) {
				ns, err := r.Seek(int64(off), io.SeekStart)
				if err != nil {
					t.Fatal(err)
				}
				if ns != int64(off) {
					t.Fatal("seek mismatch")
				}
				ok, err := r.Match(bytes.NewBuffer(content[off:off+n]), n)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Fatal("content mismatch")
				}
			})
		}
	})
}
