package cac_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestNewCAC(t *testing.T) {
	foo := "greaterthanspan"
	bmtHashOfFoo := "27913f1bdb6e8e52cbd5a5fd4ab577c857287edf6969b41efe926b51de0f4f23"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	c, err := cac.New([]byte(foo))
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}
}

func TestNewWithDataSpan(t *testing.T) {
	foo := "greaterthanspan"
	bmtHashOfFoo := "95022e6af5c6d6a564ee55a67f8455a3e18c511b5697c932d9e44f07f2fb8c53"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	c, err := cac.NewWithDataSpan([]byte(foo))
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}
}

func TestNewWithSpan(t *testing.T) {
	foo := "greaterthanspan"
	bmtHashOfFoo := "27913f1bdb6e8e52cbd5a5fd4ab577c857287edf6969b41efe926b51de0f4f23"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	span := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(span, uint64(len(foo)))

	c, err := cac.NewWithSpan([]byte(foo), span)
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}
}

var testChunkData = []struct {
	name string
	data []byte
	err  error
}{
	{
		name: "too short data chunk",
		data: []byte(strings.Repeat("a", swarm.SpanSize-1)),
		err:  cac.ErrTooShortChunkData,
	},
	{
		name: "too large data chunk",
		data: []byte(strings.Repeat("a", swarm.ChunkSize+swarm.SpanSize+1)),
		err:  cac.ErrTooLargeChunkData,
	},
}

func TestChunkInvariants(t *testing.T) {
	for _, cc := range testChunkData {
		t.Run(cc.name, func(t *testing.T) {
			_, err := cac.NewWithDataSpan(cc.data)
			if err != nil {
				if err != cc.err {
					t.Fatalf("got %v want %v", err, cc.err)
				}
			} else {
				t.Fatalf("got %v want %v", err, cc.err)
			}
		})
	}
}
