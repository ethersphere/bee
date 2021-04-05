// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

const (
	defaultMimeType     = "application/octet-stream"
	limitMetadataLength = swarm.ChunkSize
)

var (
	filename     string // flag variable, filename to use in metadata
	mimeType     string // flag variable, mime type to use in metadata
	outDir       string // flag variable, output dir for fsStore
	outFileForce bool   // flag variable, overwrite output file if exists
	host         string // flag variable, http api host
	port         int    // flag variable, http api port
	useHttp      bool   // flag variable, skips http api if not set
	ssl          bool   // flag variable, uses https for api if set
	retrieve     bool   // flag variable, if set will resolve and retrieve referenced file
	verbosity    string // flag variable, debug level
	logger       logging.Logger
)

// Entry is the underlying procedure for the CLI command
func Entry(cmd *cobra.Command, args []string) (err error) {
	return errors.New("command is deprecated")
}

func main() {
	c := &cobra.Command{
		Use:   "entry <reference>",
		Short: "Create or resolve a file entry",
		Long: `Creates a file entry, or retrieve the data referenced by the entry and its metadata.

Example:

	$ bee-file --mime-type text/plain --filename foo.txt 2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48
	> 94434d3312320fab70428c39b79dffb4abc3dbedf3e1562384a61ceaf8a7e36b
	$ bee-file --output-dir /tmp 94434d3312320fab70428c39b79dffb4abc3dbedf3e1562384a61ceaf8a7e36b
	$ cat /tmp/bar.txt

Creating a file entry:

The default file name is the hex representation of the swarm hash passed as argument, and the default mime-type is application/octet-stream. Both can be explicitly set with --filename and --mime-type respectively. If --output-dir is given, the metadata and entry chunks are written to the specified directory.

Resolving a file entry:

If --output-dir is set, the retrieved file will be written to the speficied directory. Otherwise it will be written to the current directory. Use -f to force overwriting an existing file.`,

		RunE:         Entry,
		SilenceUsage: true,
	}

	c.Flags().StringVar(&filename, "filename", "", "filename to use in entry")
	c.Flags().StringVar(&mimeType, "mime-type", "", "mime-type to use in collection")
	c.Flags().BoolVarP(&outFileForce, "force", "f", false, "overwrite existing output file")
	c.Flags().StringVarP(&outDir, "output-dir", "d", "", "save directory")
	c.Flags().StringVar(&host, "host", "127.0.0.1", "api host")
	c.Flags().IntVar(&port, "port", 1633, "api port")
	c.Flags().BoolVar(&ssl, "ssl", false, "use ssl")
	c.Flags().BoolVarP(&retrieve, "retrieve", "r", false, "retrieve file from referenced entry")
	c.Flags().BoolVar(&useHttp, "http", false, "save entry to bee http api")
	c.Flags().StringVar(&verbosity, "info", "0", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")

	c.SetOutput(c.OutOrStdout())
	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
