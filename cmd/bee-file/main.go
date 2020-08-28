// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	cmdfile "github.com/ethersphere/bee/cmd/internal/file"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/file/seekjoiner"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
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

// getEntry handles retrieving and writing a file from the file entry
// referenced by the given address.
func getEntry(cmd *cobra.Command, args []string) (err error) {
	// process the reference to retrieve
	addr, err := swarm.ParseHexAddress(args[0])
	if err != nil {
		return err
	}

	// initialize interface with HTTP API
	store := cmdfile.NewApiStore(host, port, ssl)

	buf := bytes.NewBuffer(nil)
	writeCloser := cmdfile.NopWriteCloser(buf)
	limitBuf := cmdfile.NewLimitWriteCloser(writeCloser, limitMetadataLength)
	j := seekjoiner.NewSimpleJoiner(store)
	_, err = file.JoinReadAll(cmd.Context(), j, addr, limitBuf)
	if err != nil {
		return err
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		return err
	}

	buf = bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(cmd.Context(), j, e.Metadata(), buf)
	if err != nil {
		return err
	}

	// retrieve metadata
	metaData := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metaData)
	if err != nil {
		return err
	}
	logger.Debugf("Filename: %s", metaData.Filename)
	logger.Debugf("MIME-type: %s", metaData.MimeType)

	if outDir == "" {
		outDir = "."
	} else {
		err := os.MkdirAll(outDir, 0o777) // skipcq: GSC-G301
		if err != nil {
			return err
		}
	}
	outFilePath := filepath.Join(outDir, metaData.Filename)

	// create output dir if not exist
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
	outFile, err := os.OpenFile(outFilePath, outFileFlags, 0o666) // skipcq: GSC-G302
	if err != nil {
		return err
	}
	defer outFile.Close()
	_, err = file.JoinReadAll(cmd.Context(), j, e.Reference(), outFile)
	return err
}

// putEntry creates a new file entry with the given reference.
func putEntry(cmd *cobra.Command, args []string) (err error) {
	// process the reference to retrieve
	addr, err := swarm.ParseHexAddress(args[0])
	if err != nil {
		return err
	}
	// add the fsStore and/or apiStore, depending on flags
	stores := cmdfile.NewTeeStore()
	if outDir != "" {
		err := os.MkdirAll(outDir, 0o777) // skipcq: GSC-G301
		if err != nil {
			return err
		}
		store := cmdfile.NewFsStore(outDir)
		stores.Add(store)
	}
	if useHttp {
		store := cmdfile.NewApiStore(host, port, ssl)
		stores.Add(store)
	}

	// create metadata object, with defaults for missing values
	if filename == "" {
		filename = args[0]
	}
	if mimeType == "" {
		mimeType = defaultMimeType
	}
	metadata := entry.NewMetadata(filename)
	metadata.MimeType = mimeType

	// serialize metadata and send it to splitter
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	logger.Debugf("metadata contents: %s", metadataBytes)

	// set up splitter to process the metadata
	s := splitter.NewSimpleSplitter(stores, storage.ModePutUpload)
	ctx := context.Background()

	// first add metadata
	metadataBuf := bytes.NewBuffer(metadataBytes)
	metadataReader := io.LimitReader(metadataBuf, int64(len(metadataBytes)))
	metadataReadCloser := ioutil.NopCloser(metadataReader)
	metadataAddr, err := s.Split(ctx, metadataReadCloser, int64(len(metadataBytes)), false)
	if err != nil {
		return err
	}

	// create entry from given reference and metadata,
	// serialize and send to splitter
	fileEntry := entry.New(addr, metadataAddr)
	fileEntryBytes, err := fileEntry.MarshalBinary()
	if err != nil {
		return err
	}
	fileEntryBuf := bytes.NewBuffer(fileEntryBytes)
	fileEntryReader := io.LimitReader(fileEntryBuf, int64(len(fileEntryBytes)))
	fileEntryReadCloser := ioutil.NopCloser(fileEntryReader)
	fileEntryAddr, err := s.Split(ctx, fileEntryReadCloser, int64(len(fileEntryBytes)), false)
	if err != nil {
		return err
	}

	// output reference to file entry
	cmd.Println(fileEntryAddr)
	return nil
}

// Entry is the underlying procedure for the CLI command
func Entry(cmd *cobra.Command, args []string) (err error) {
	logger, err = cmdfile.SetLogger(cmd, verbosity)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if retrieve {
		return getEntry(cmd, args)
	}
	return putEntry(cmd, args)
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
	c.Flags().IntVar(&port, "port", 8080, "api port")
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
