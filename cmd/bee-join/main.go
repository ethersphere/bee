// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	cmdfile "github.com/ethersphere/bee/cmd/internal/file"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/seekjoiner"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

var (
	outFilePath  string // flag variable, output file
	outFileForce bool   // flag variable, overwrite output file if exists
	host         string // flag variable, http api host
	port         int    // flag variable, http api port
	ssl          bool   // flag variable, uses https for api if set
	indir        string // flag variable, directory to retrieve chunks from
	verbosity    string // flag variable, debug level
	logger       logging.Logger
)

// Join is the underlying procedure for the CLI command
func Join(cmd *cobra.Command, args []string) (err error) {
	logger, err = cmdfile.SetLogger(cmd, verbosity)
	if err != nil {
		return err
	}

	// if output file is specified, create it if it does not exist
	var outFile *os.File
	if outFilePath != "" {
		// make sure we have full path
		outDir := filepath.Dir(outFilePath)
		if outDir != "." {
			err := os.MkdirAll(outDir, 0o777) // skipcq: GSC-G301
			if err != nil {
				return err
			}
		}
		// protect any existing file unless explicitly told not to
		outFileFlags := os.O_CREATE | os.O_WRONLY
		if outFileForce {
			outFileFlags |= os.O_TRUNC
		} else {
			outFileFlags |= os.O_EXCL
		}
		// open the file
		outFile, err = os.OpenFile(outFilePath, outFileFlags, 0o666) // skipcq: GSC-G302
		if err != nil {
			return err
		}
		defer outFile.Close()
		logger.Debugf("writing to %s", outFilePath)
	} else {
		outFile = os.Stdout
		logger.Debugf("writing to stdout")
	}

	// process the reference to retrieve
	addr, err := swarm.ParseHexAddress(args[0])
	if err != nil {
		return err
	}

	// initialize interface with backend store
	// either from directory if set, or HTTP API if not set
	var store storage.Getter
	if indir != "" {
		store = cmdfile.NewFsStore(indir)
	} else {
		store = cmdfile.NewApiStore(host, port, ssl)
	}

	// create the join and get its data reader
	j := seekjoiner.NewSimpleJoiner(store)
	_, err = file.JoinReadAll(cmd.Context(), j, addr, outFile)
	return err
}

func main() {
	c := &cobra.Command{
		Use:   "join [hash]",
		Args:  cobra.ExactArgs(1),
		Short: "Retrieve data from Swarm",
		Long: `Assembles chunked data from referenced by a root Swarm Hash.

Will output retrieved data to stdout.`,
		RunE:         Join,
		SilenceUsage: true,
	}
	c.Flags().StringVarP(&outFilePath, "output-file", "o", "", "file to write output to")
	c.Flags().BoolVarP(&outFileForce, "force", "f", false, "overwrite existing output file")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 8080, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")
	c.Flags().StringVarP(&indir, "input-dir", "i", "", "retrieve chunks from directory")
	c.Flags().StringVar(&verbosity, "info", "0", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")

	c.SetOutput(c.OutOrStdout())
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
