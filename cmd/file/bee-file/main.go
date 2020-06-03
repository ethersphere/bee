// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"encoding/json"
	"os"
	"path/filepath"

	cmdfile "github.com/ethersphere/bee/cmd/file"
	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file/splitter"
	"github.com/ethersphere/bee/pkg/file/joiner"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/cobra"
)

const (
	defaultMimeType = "application/octet-stream"
	limitMetadataLength = 1024 * 1024
)

var (
	filename string
	mimeType string
	outDir      string // flag variable, output dir for fsStore
	outFileForce bool   // flag variable, overwrite output file if exists
	host        string // flag variable, http api host
	port        int    // flag variable, http api port
	noHttp      bool   // flag variable, skips http api if set
	ssl         bool   // flag variable, uses https for api if set
	retrieve	bool 
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

	// TODO: need to limit writer
	buf := bytes.NewBuffer(nil)
	limitBuf := cmdfile.NewLimitWriteCloser(buf, func() error { return nil }, limitMetadataLength)
	j := joiner.NewSimpleJoiner(store)
	err = cmdfile.JoinReadAll(j, addr, limitBuf)
	if err != nil {
		return err
	}
	e := &entry.Entry{}
	err = e.UnmarshalBinary(buf.Bytes())
	if err != nil {
		return err
	}
	fmt.Println(e)

	buf = bytes.NewBuffer(nil)
	err = cmdfile.JoinReadAll(j, e.Metadata(), buf)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n\n", buf.Bytes())

	// retrieve metadata
	metaData := &entry.Metadata{}
	err = json.Unmarshal(buf.Bytes(), metaData)
	if err != nil {
		return err
	}

	if outDir == "" {
		outDir = "."
	} else {
		err := os.MkdirAll(outDir, 0o777) // skipcq: GSC-G301
		if err != nil {
			return err
		}
	}
	outFilePath := filepath.Join(outDir, metaData.Filename)
	mimeType := metaData.MimeType
	fmt.Fprintf(os.Stderr, "mime type %s", mimeType)

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

	fmt.Fprintf(os.Stderr, "reference %s", e.Reference())
	err = cmdfile.JoinReadAll(j, e.Reference(), outFile)
	if err != nil {
		return err
	}

	return nil
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
	if !noHttp {
		store := cmdfile.NewApiStore(host, port, ssl)
		stores.Add(store)
	}

	s := splitter.NewSimpleSplitter(stores)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// create metadata object, with defaults for missing values
	if filename == "" {
		filename = args[0]
	}
	if mimeType == "" {
		mimeType = defaultMimeType
	}
	metadata := entry.NewMetadata(filename)
	metadata.MimeType =mimeType

	// serialize metadata and send it to splitter
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	metadataBuf := bytes.NewBuffer(metadataBytes)
	metadataReader := cmdfile.NewLimitReadCloser(metadataBuf, func() error { return nil }, int64(len(metadataBytes)))
	metadataAddr, err := s.Split(ctx, metadataReader, int64(len(metadataBytes)))
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
	fileEntryReader := cmdfile.NewLimitReadCloser(fileEntryBuf, func() error { return nil }, int64(len(fileEntryBytes)))
	fileEntryAddr, err := s.Split(ctx, fileEntryReader, int64(len(fileEntryBytes)))
	if err != nil {
		return err
	}

	// output reference to file entry
	fmt.Println(fileEntryAddr)
	return nil
}

// Entry is the underlying procedure for the CLI command
func Entry(cmd *cobra.Command, args []string) (err error) {
	if retrieve {
		return getEntry(cmd, args)
	}
	return putEntry(cmd, args)
}


func main() {
	c := &cobra.Command{
		Use:   "entry <reference>",
		Short: "Create a file entry",
		RunE:	Entry,
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
	c.Flags().BoolVar(&noHttp, "no-http", false, "skip http put")

	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
