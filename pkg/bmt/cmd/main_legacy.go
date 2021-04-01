// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command main_legacy executes the BMT hash algorithm on the given data and writes the binary result to standard output
//
// Up to 4096 bytes will be read
//
// If a filename is given as argument, it reads data from the file. Otherwise it reads data from standard input.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/ethersphere/bee/pkg/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

func main() {
	var data [4096]byte
	var err error
	var infile *os.File

	if len(os.Args) > 1 {
		infile, err = os.Open(os.Args[1])
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
	} else {
		infile = os.Stdin
	}
	var c int
	c, err = infile.Read(data[:])

	// EOF means zero-length input. This is still valid input for BMT
	if err != nil && err != io.EOF {
		fmt.Fprint(os.Stderr, err.Error())
		infile.Close()
		os.Exit(1)
	}
	infile.Close()

	hashPool := legacy.NewTreePool(sha3.NewLegacyKeccak256, 128, legacy.PoolSize)
	bmtHash := legacy.New(hashPool)
	_, err = bmtHash.Write(data[:c])
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
	binSum := bmtHash.Sum(nil)
	_, err = os.Stdout.Write(binSum)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(1)
	}
}
