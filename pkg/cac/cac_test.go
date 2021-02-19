package cac_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewCAC(t *testing.T) {
	data := []byte("greaterthanspan")
	bmtHashOfData := "27913f1bdb6e8e52cbd5a5fd4ab577c857287edf6969b41efe926b51de0f4f23"
	address := swarm.MustParseHexAddress(bmtHashOfData)

	expectedSpan := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(expectedSpan, uint64(len(data)))
	expectedContent := append(expectedSpan, data...)

	c, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}

	if !bytes.Equal(c.Data(), expectedContent) {
		t.Fatalf("chunk data mismatch. got %x want %x", c.Data(), expectedContent)
	}
}

func TestNewWithDataSpan(t *testing.T) {
	data := []byte("greaterthanspan")
	bmtHashOfData := "95022e6af5c6d6a564ee55a67f8455a3e18c511b5697c932d9e44f07f2fb8c53"
	address := swarm.MustParseHexAddress(bmtHashOfData)

	c, err := cac.NewWithDataSpan(data)
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}

	if !bytes.Equal(c.Data(), data) {
		t.Fatalf("chunk data mismatch. got %x want %x", c.Data(), data)
	}
}

func TestChunkInvariants(t *testing.T) {
	chunkerFunc := []struct {
		name    string
		chunker func(data []byte) (swarm.Chunk, error)
	}{
		{
			name: "new cac",
			chunker: func(data []byte) (swarm.Chunk, error) {
				return cac.New(data)
			},
		},
		{
			name: "new chunk with data span",
			chunker: func(data []byte) (swarm.Chunk, error) {
				return cac.NewWithDataSpan(data)
			},
		},
	}

	for _, f := range chunkerFunc {
		for _, cc := range []struct {
			name    string
			data    []byte
			wantErr error
		}{
			{
				name:    "too short data chunk",
				data:    []byte("a"),
				wantErr: cac.ErrTooShortChunkData,
			},
			{
				name:    "too large data chunk",
				data:    []byte(strings.Repeat("a", swarm.ChunkSize+swarm.SpanSize+1)),
				wantErr: cac.ErrTooLargeChunkData,
			},
		} {
			testName := fmt.Sprintf("%s-%s", f.name, cc.name)
			t.Run(testName, func(t *testing.T) {
				_, err := f.chunker(cc.data)
				if err != nil {
					if err != cc.wantErr {
						t.Fatalf("got %v want %v", err, cc.wantErr)
					}
				} else {
					t.Fatalf("got %v want %v", err, cc.wantErr)
				}
			})
		}
	}
}
