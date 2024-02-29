// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bufio"
	crand "crypto/rand"
	"math/rand"
	"os"
	"path"
	"testing"

	"github.com/ethersphere/bee/cmd/bee/cmd"
	"github.com/ethersphere/bee/pkg/api"
)

func TestDBSplitRefs(t *testing.T) {
	t.Parallel()

	s := (rand.Intn(10) + 10) * 1024 // rand between 10 and 20 KB
	buf := make([]byte, s)
	_, err := crand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	inputFileName := path.Join(t.TempDir(), "input")
	err = os.WriteFile(inputFileName, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	outputFileName := path.Join(t.TempDir(), "output")

	err = newCommand(t, cmd.WithArgs("split", "refs", "--input-file", inputFileName, "--output-file", outputFileName)).Execute()
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
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		gotHashes++
	}

	if gotHashes != wantHashes {
		t.Fatalf("got %d hashes, want %d", gotHashes, wantHashes)
	}
}

func TestDBSplitChunks(t *testing.T) {
	t.Parallel()

	s := (rand.Intn(10) + 10) * 1024 // rand between 10 and 20 KB
	buf := make([]byte, s)
	_, err := crand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	inputFileName := path.Join(t.TempDir(), "input")
	err = os.WriteFile(inputFileName, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}

	dir := path.Join(t.TempDir(), "chunks")
	err = os.Mkdir(dir, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	err = newCommand(t, cmd.WithArgs("split", "chunks", "--input-file", inputFileName, "--output-dir", dir, "--r-level", "3")).Execute()
	if err != nil {
		t.Fatal(err)
	}

	stat, err := os.Stat(inputFileName)
	if err != nil {
		t.Fatal(err)
	}
	want := api.CalculateNumberOfChunks(stat.Size(), false)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if int64(len(entries)) < want {
		t.Fatalf("want at least %d chunks", want)
	}

}
