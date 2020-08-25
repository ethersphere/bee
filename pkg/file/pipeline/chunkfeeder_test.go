package pipeline

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestFeeder(t *testing.T) {
	var (
		chunkSize = 5
		data      = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	)

	for _, tc := range []struct {
		name      string
		dataSize  []int // how big each write is
		expWrites int
		writeData []byte
	}{
		{
			name:      "empty write",
			dataSize:  []int{0},
			expWrites: 0,
		},
		{
			name:      "less than chunk, no writes",
			dataSize:  []int{3},
			expWrites: 0,
		},
		{
			name:      "one chunk, one write",
			dataSize:  []int{5},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
		},
		{
			name:      "two chunks, two writes",
			dataSize:  []int{10},
			expWrites: 2,
			writeData: []byte{6, 7, 8, 9, 10},
		},
		{
			name:      "half chunk, then full one, one write",
			dataSize:  []int{3, 5},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
		},

		{
			name:      "half chunk, another two halves, one write",
			dataSize:  []int{3, 2, 3},
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var results pipeWriteArgs
			rr := newResultWriter(&results)
			cf := NewChunkFeederWriter(chunkSize, rr)
			i := 0
			for _, v := range tc.dataSize {
				d := data[i : i+v]
				fmt.Println("test writing", d)
				n, err := cf.Write(d)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Println("wrote bytes", n)
				if n != v {
					t.Fatalf("wrote %d bytes but expected %d bytes", n, v)
				}
				i += v
			}

			if tc.expWrites == 0 && results.data != nil {
				t.Fatal("expected no write but got one")
			}

			if rr.count != tc.expWrites {
				t.Fatalf("expected %d writes but got %d", tc.expWrites, rr.count)
			}
			fmt.Println("result", results.data)

			if results.data != nil && !bytes.Equal(tc.writeData, results.data[8:]) {
				t.Fatalf("expected write data %v but got %v", tc.writeData, results.data[8:])
			}
		})
	}
}

type countingResultWriter struct {
	target *pipeWriteArgs
	count  int
}

func newResultWriter(b *pipeWriteArgs) *countingResultWriter {
	return &countingResultWriter{target: b}
}

func (w *countingResultWriter) ChainWrite(p *pipeWriteArgs) (int, error) {
	w.count++
	*w.target = *p
	return 0, nil
}

func (w *countingResultWriter) Sum() ([]byte, error) {
	return nil, errors.New("not implemented")
}
