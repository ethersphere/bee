package cac_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
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
			name:    "new cac",
			chunker: cac.New,
		},
		{
			name:    "new chunk with data span",
			chunker: cac.NewWithDataSpan,
		},
	}

	for _, f := range chunkerFunc {
		for _, cc := range []struct {
			name    string
			data    []byte
			wantErr error
		}{
			{
				name:    "zero data",
				data:    []byte{},
				wantErr: cac.ErrTooShortChunkData,
			},
			{
				name:    "nil",
				data:    nil,
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
				if !errors.Is(err, cc.wantErr) {
					t.Fatalf("got %v want %v", err, cc.wantErr)
				}
			})
		}
	}
}

// TestValid checks whether a chunk is a valid content-addressed chunk
func TestValid(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	foo := "foo"
	fooLength := len(foo)
	fooBytes := make([]byte, swarm.SpanSize+fooLength)
	binary.LittleEndian.PutUint64(fooBytes, uint64(fooLength))
	copy(fooBytes[swarm.SpanSize:], foo)
	ch := swarm.NewChunk(address, fooBytes)
	if !cac.Valid(ch) {
		t.Fatalf("data '%s' should have validated to hash '%s'", ch.Data(), ch.Address())
	}
}

/// TestInvalid checks whether a chunk is not a valid content-addressed chunk
func TestInvalid(t *testing.T) {
	// Generates a chunk with the given data. No validation is performed here,
	// the chunks are create as it is.
	chunker := func(addr string, dataBytes []byte) swarm.Chunk {
		// Decoding errors are ignored here since they will be captured
		// when validating
		addrBytes, _ := hex.DecodeString(addr)
		address := swarm.NewAddress(addrBytes)
		return swarm.NewChunk(address, dataBytes)
	}

	// Appends span to given input data.
	dataWithSpan := func(inputData []byte) []byte {
		dataLength := len(inputData)
		dataBytes := make([]byte, swarm.SpanSize+dataLength)
		binary.LittleEndian.PutUint64(dataBytes, uint64(dataLength))
		copy(dataBytes[swarm.SpanSize:], inputData)
		return dataBytes
	}

	for _, c := range []struct {
		name    string
		address string
		data    []byte
	}{
		{
			name:    "wrong address",
			address: "0000e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48",
			data:    dataWithSpan([]byte("foo")),
		},
		{
			name:    "empty address",
			address: "",
			data:    dataWithSpan([]byte("foo")),
		},
		{
			name:    "zero data",
			address: "anything",
			data:    []byte{},
		},
		{
			name:    "nil data",
			address: "anything",
			data:    nil,
		},
		{
			name:    "small data",
			address: "6251dbc53257832ae80d0e9f1cc41bd54d5b6c704c9c7349709c07fefef0aea6",
			data:    []byte("small"),
		},
		{
			name:    "large data",
			address: "ffd70157e48063fc33c97a050f7f640233bf646cc98d9524c6b92bcf3ab56f83",
			data:    []byte(strings.Repeat("a", swarm.ChunkSize+swarm.SpanSize+1)),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			ch := chunker(c.address, c.data)
			if cac.Valid(ch) {
				t.Fatalf("data '%s' should not have validated to hash '%s'", ch.Data(), ch.Address())
			}
		})
	}
}
