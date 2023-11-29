// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bufio"
	crand "crypto/rand"
	"errors"
	"io"
	"math/rand"
	"os"
	"testing"

	"github.com/ethersphere/bee/cmd/bee/cmd"
	"github.com/ethersphere/bee/pkg/api"
)

func TestDBSplit(t *testing.T) {
	t.Parallel()

	s := (rand.Intn(10) + 10) * 1024 // rand between 10 and 20 KB
	buf := make([]byte, s)
	_, err := crand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	inputFileName := t.TempDir() + "/input"
	err = os.WriteFile(inputFileName, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	outputFileName := t.TempDir() + "/output"

	err = newCommand(t, cmd.WithArgs("split", "--input-file", inputFileName, "--output-file", outputFileName)).Execute()
	if err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(inputFileName)
	if err != nil {
		t.Fatal(err)
	}
	wantHashes := api.CalculateNumberOfChunks(stat.Size(), false) + 1 // +1 for the root hash
	var gotHashes int64

	f, err := os.Open(outputFileName)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		_, err = reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatal(err)
		}
		gotHashes++
	}

	if gotHashes != wantHashes {
		t.Fatalf("got %d hashes, want %d", gotHashes, wantHashes)
	}
}
