// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package langos_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/langos"
)

// TestBufferedReadSeeker runs a series of reads and seeks on
// BufferedReadSeeker instances with various buffer sizes.
func TestBufferedReadSeeker(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)
		newReadSeekerTester(langos.NewBufferedReadSeeker(bytes.NewReader(data), bufferSize), data)(t)
	})
}

// TestLangosReadSeeker runs a series of reads and seeks on
// Langos instances with various buffer sizes.
func TestLangosReadSeeker(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)
		newReadSeekerTester(langos.NewLangos(bytes.NewReader(data), bufferSize), data)(t)
	})
}

// TestBufferedLangosReadSeeker runs a series of reads and seeks on
// buffered Langos instances with various buffer sizes.
func TestBufferedLangosReadSeeker(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)
		newReadSeekerTester(langos.NewBufferedLangos(bytes.NewReader(data), bufferSize), data)(t)
	})
}

// TestReadSeekerTester tests newReadSeekerTester steps against the stdlib's
// bytes.Reader which is used as the reference implementation.
func TestReadSeekerTester(t *testing.T) {
	for _, size := range testDataSizes {
		data := randomData(t, parseDataSize(t, size))
		t.Run(size, newReadSeekerTester(bytes.NewReader(data), data))
	}
}

// newReadSeekerTester returns a new test function that performs a series of
// Read and Seek method calls to validate that provided io.ReadSeeker
// provide the expected functionality while reading data and seeking on it.
// Argument data must be the same as used in io.ReadSeeker as it is used
// in validations.
func newReadSeekerTester(rs io.ReadSeeker, data []byte) func(t *testing.T) {
	return func(t *testing.T) {
		read := func(t *testing.T, size int, want []byte, wantErr error) {
			t.Helper()

			b := make([]byte, size)
			for count := 0; count < len(want); {
				n, err := rs.Read(b[count:])
				if err != wantErr {
					t.Fatalf("got error %v, want %v", err, wantErr)
				}
				count += n
			}
			if !bytes.Equal(b, want) {
				t.Fatal("invalid read data")
			}
		}

		seek := func(t *testing.T, offset, whence, wantPosition int, wantErr error) {
			t.Helper()

			n, err := rs.Seek(int64(offset), whence)
			if err != wantErr {
				t.Fatalf("got error %v, want %v", err, wantErr)
			}
			if n != int64(wantPosition) {
				t.Fatalf("got position %v, want %v", n, wantPosition)
			}
		}

		l := len(data)

		// Test sequential reads
		readSize1 := l / 5
		read(t, readSize1, data[:readSize1], nil)
		readSize2 := l / 6
		read(t, readSize2, data[readSize1:readSize1+readSize2], nil)
		readSize3 := l / 4
		read(t, readSize3, data[readSize1+readSize2:readSize1+readSize2+readSize3], nil)

		// Test seek and read
		seekSize1 := l / 4
		seek(t, seekSize1, io.SeekStart, seekSize1, nil)
		readSize1 = l / 5
		read(t, readSize1, data[seekSize1:seekSize1+readSize1], nil)
		readSize2 = l / 10
		read(t, readSize2, data[seekSize1+readSize1:seekSize1+readSize1+readSize2], nil)

		// Test get size and read from start
		seek(t, 0, io.SeekEnd, l, nil)
		seek(t, 0, io.SeekStart, 0, nil)
		readSize1 = l / 6
		read(t, readSize1, data[:readSize1], nil)

		// Test read end
		seek(t, 0, io.SeekEnd, l, nil)
		read(t, 0, nil, io.EOF)

		// Test read near end
		seekOffset := 1 / 10
		seek(t, seekOffset, io.SeekEnd, l-seekOffset, nil)
		read(t, seekOffset, data[l-seekOffset:], io.EOF)

		// Test seek from current with reads
		seek(t, 0, io.SeekStart, 0, nil)
		seekSize1 = l / 3
		seek(t, seekSize1, io.SeekCurrent, seekSize1, nil)
		readSize1 = l / 8
		read(t, readSize1, data[seekSize1:seekSize1+readSize1], nil)

	}
}
