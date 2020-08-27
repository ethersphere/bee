// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	test "github.com/ethersphere/bee/pkg/file/testing"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestPartialWrites(t *testing.T) {
	m := mock.NewStorer()
	p := pipeline.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)
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
	p := pipeline.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

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
			p := pipeline.NewPipelineBuilder(context.Background(), m, storage.ModePutUpload, false)

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
