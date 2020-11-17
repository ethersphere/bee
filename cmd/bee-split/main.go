// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	cmdfile "github.com/ethersphere/bee/cmd/internal/file"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/spf13/cobra"
)

var (
	outdir      string // flag variable, output dir for fsStore
	inputLength int64  // flag variable, limit of data input
	host        string // flag variable, http api host
	port        int    // flag variable, http api port
	useHttp     bool   // flag variable, skips http api if not set
	ssl         bool   // flag variable, uses https for api if set
	verbosity   string // flag variable, debug level
	logger      logging.Logger
)

// Split is the underlying procedure for the CLI command
func Split(cmd *cobra.Command, args []string) (err error) {
	logger, err = cmdfile.SetLogger(cmd, verbosity)
	if err != nil {
		return err
	}

	// if one arg is set, this is the input file
	// if not, we are reading from standard input
	var infile io.ReadCloser
	if len(args) > 0 {

		// get the file length
		info, err := os.Stat(args[0])
		if err != nil {
			return err
		}
		fileLength := info.Size()

		// check if we are limiting the input, and if the limit is valid
		if inputLength > 0 {
			if inputLength > fileLength {
				return fmt.Errorf("input data length set to %d on file with length %d", inputLength, fileLength)
			}
		} else {
			inputLength = fileLength
		}

		// open file and wrap in limiter
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		fileReader := io.LimitReader(f, inputLength)
		infile = ioutil.NopCloser(fileReader)
		logger.Debugf("using %d bytes from file %s as input", fileLength, args[0])
	} else {
		// this simple splitter is too stupid to handle open-ended input, sadly
		if inputLength == 0 {
			return errors.New("must specify length of input on stdin")
		}
		stdinReader := io.LimitReader(os.Stdin, inputLength)
		infile = ioutil.NopCloser(stdinReader)
		logger.Debugf("using %d bytes from standard input", inputLength)
	}

	// add the fsStore and/or apiStore, depending on flags
	stores := cmdfile.NewTeeStore()
	if outdir != "" {
		err := os.MkdirAll(outdir, 0o777) // skipcq: GSC-G301
		if err != nil {
			return err
		}
		store := cmdfile.NewFsStore(outdir)
		stores.Add(store)
		logger.Debugf("using directory %s for output", outdir)
	}
	if useHttp {
		store := cmdfile.NewApiStore(host, port, ssl)
		stores.Add(store)
		logger.Debugf("using bee http (ssl=%v) api on %s:%d for output", ssl, host, port)
	}

	// split and rule
	s := splitter.NewSimpleSplitter(stores, storage.ModePutUpload)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := s.Split(ctx, infile, inputLength, false)
	if err != nil {
		return err
	}

	// output the resulting hash
	cmd.Println(addr)
	return nil
}

func main() {
	c := &cobra.Command{
		Use:   "split [datafile]",
		Args:  cobra.RangeArgs(0, 1),
		Short: "Split data into swarm chunks",
		Long: `Creates and stores Swarm chunks from input data.

If datafile is not given, data will be read from standard in. In this case the --count flag must be set 
to the length of the input.

The application will expect to transmit the chunks to the bee HTTP API, unless the --no-http flag has been set.

If --output-dir is set, the chunks will be saved to the file system, using the flag argument as destination directory. 
Chunks are saved in individual files, and the file names will be the hex addresses of the chunks.`,
		RunE:         Split,
		SilenceUsage: true,
	}

	c.Flags().StringVarP(&outdir, "output-dir", "d", "", "saves chunks to given directory")
	c.Flags().Int64VarP(&inputLength, "count", "c", 0, "read at most this many bytes")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 1633, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")
	c.Flags().BoolVar(&useHttp, "http", false, "save chunks to bee http api")
	c.Flags().StringVar(&verbosity, "info", "0", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")

	c.SetOutput(c.OutOrStdout())
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
