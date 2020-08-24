package pipeline

import (
	"errors"
	"testing"
)

func TestFeeder(t *testing.T) {
	var (
		chunkSize = 5
		data      = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	)

	for _, tc := range []struct {
		name      string
		dataSize  int
		expWrites int
		writeData []byte
	}{
		{
			name:      "empty write",
			dataSize:  0,
			expWrites: 0,
		},
		{
			name:      "less than chunk, no writes",
			dataSize:  3,
			expWrites: 0,
		},
		{
			name:      "one chunk, one write",
			dataSize:  5,
			expWrites: 1,
			writeData: []byte{1, 2, 3, 4, 5},
		},
		{
			name:      "two chunks, two writes",
			dataSize:  10,
			expWrites: 2,
			writeData: []byte{6, 7, 8, 9, 10},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var results pipeWriteArgs
			rr := newResultWriter(&results)
			cf := NewChunkFeederWriter(chunkSize, rr)
			d := data[:tc.dataSize]
			n, err := cf.Write(d)
			if err != nil {
				t.Fatal(err)
			}
			if n != tc.dataSize {
				t.Fatalf("wrote %d bytes but expected %d bytes", n, tc.dataSize)
			}

			if tc.expWrites == 0 && results.data != nil {
				t.Fatal("expected no write but got one")
			}

			if rr.count != tc.expWrites {
				t.Fatalf("expected %d writes but got %d", tc.expWrites, rr.count)
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
