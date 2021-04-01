// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command generate_legacy generates bmt hashes of sequential byte inputs
// for every possible length of legacy bmt hasher
package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/ethersphere/bee/pkg/bmt/legacy"
	"gitlab.com/nolash/go-mockbytes"
	"golang.org/x/crypto/sha3"
)

func main() {

	// create output directory, fail if it already exists or error creating
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: generate-hashes <output_directory>\n")
		os.Exit(1)
	}
	outputDir, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid input: %s", err)
		os.Exit(1)
	}
	err = os.Mkdir(outputDir, 0750)
	if err == os.ErrExist {
		fmt.Fprintf(os.Stderr, "Directory %s already exists\n", outputDir)
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// set up hasher
	hashPool := legacy.NewTreePool(sha3.NewLegacyKeccak256, 128, legacy.PoolSize)
	bmtHash := legacy.New(hashPool)

	// create sequence generator and outputs
	var i int
	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	for i = 0; i < 4096; i++ {
		s := fmt.Sprintf("processing %d...", i)
		fmt.Fprintf(os.Stderr, "%-64s\r", s)
		filename := fmt.Sprintf("%s/%d.bin", outputDir, i)
		b, err := g.SequentialBytes(i)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		bmtHash.Reset()
		_, err = bmtHash.Write(b)
		sum := bmtHash.Sum(nil)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		err = ioutil.WriteFile(filename, sum, 0666)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
			os.Exit(1)
		}
		err = ioutil.WriteFile(filename, b, 0666)
		if err != nil {
			fmt.Fprint(os.Stderr, err.Error())
		}
	}

	// Be kind and give feedback to user
	dirString := fmt.Sprintf("Done. Data is in %s. Enjoy!", outputDir)
	fmt.Printf("%-64s\n", dirString)
}
