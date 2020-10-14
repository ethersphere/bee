// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package builder_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"gitlab.com/nolash/go-mockbytes"
)

func TestPartialWrites(t *testing.T) {
	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)
	_, _ = p.Write([]byte("hello "))
	_, _ = p.Write([]byte("world"))

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestHelloWorld(t *testing.T) {
	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

	data := []byte("hello world")
	_, err := p.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := p.Sum()
	if err != nil {
		t.Fatal(err)
	}
	exp := swarm.MustParseHexAddress("92672a471f4419b255d7cb0cf313474a6f5856fb347c5ece85fb706d644b630f")
	if !bytes.Equal(exp.Bytes(), sum) {
		t.Fatalf("expected %s got %s", exp.String(), hex.EncodeToString(sum))
	}
}

func TestAllVectors(t *testing.T) {
	for i := 1; i <= 20; i++ {
		data, expect := test.GetVector(t, i)
		t.Run(fmt.Sprintf("data length %d, vector %d", len(data), i), func(t *testing.T) {
			m := mock.NewStorer()
			p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

			_, err := p.Write(data)
			if err != nil {
				t.Fatal(err)
			}
			sum, err := p.Sum()
			if err != nil {
				t.Fatal(err)
			}
			a := swarm.NewAddress(sum)
			if !a.Equal(expect) {
				t.Fatalf("failed run %d, expected address %s but got %s", i, expect.String(), a.String())
			}
		})
	}
}

func BenchmarkPipeline(b *testing.B) {
	for _, count := range []int{
		1000,      // 1k
		10000,     // 10 k
		100000,    // 100 k
		1000000,   // 1 meg
		10000000,  // 10 megs
		100000000, // 100 megs
	} {
		b.Run(strconv.Itoa(count)+"-bytes", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				benchmarkPipeline(b, count)
			}
		})
	}
}

func benchmarkPipeline(b *testing.B, count int) {
	b.StopTimer()

	m := mock.NewStorer()
	p := builder.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	data, err := g.SequentialBytes(count)
	if err != nil {
		b.Fatal(err)
	}
	b.StartTimer()

	_, err = p.Write(data)
	if err != nil {
		b.Fatal(err)
	}
	_, err = p.Sum()
	if err != nil {
		b.Fatal(err)
	}

}
