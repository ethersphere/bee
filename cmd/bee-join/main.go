// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/file/joiner"
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
)

// apiStore provies a storage.Getter that retrieves chunks from swarm through the HTTP chunk API.
type apiStore struct {
	baseUrl string
}

// newApiStore creates a new apiStore
func newApiStore(host string, port int, ssl bool) storage.Getter {
	scheme := "http"
	if ssl {
		scheme += "s"
	}
	u := &url.URL{
		Host:   fmt.Sprintf("%s:%d", host, port),
		Scheme: scheme,
		Path:   "bzz-chunk",
	}
	return &apiStore{
		baseUrl: u.String(),
	}
}

// Get implements storage.Getter
func (a *apiStore) Get(ctx context.Context, mode storage.ModeGet, address swarm.Address) (ch swarm.Chunk, err error) {
	addressHex := address.String()
	url := strings.Join([]string{a.baseUrl, addressHex}, "/")
	res, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chunk %s not found", addressHex)
	}
	chunkData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	ch = swarm.NewChunk(address, chunkData)
	return ch, nil
}

// Join is the underlying procedure for the CLI command
func Join(cmd *cobra.Command, args []string) (err error) {
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
	} else {
		outFile = os.Stdout
	}

	// process the reference to retrieve
	addr, err := swarm.ParseHexAddress(args[0])
	if err != nil {
		return err
	}

	// initialize interface with HTTP API
	store := newApiStore(host, port, ssl)

	// create the join and get its data reader
	j := joiner.NewSimpleJoiner(store)
	r, l, err := j.Join(context.Background(), addr)
	if err != nil {
		return err
	}

	// join, rinse, repeat until done
	data := make([]byte, swarm.ChunkSize)
	var total int64
	for i := int64(0); i < l; i += swarm.ChunkSize {
		cr, err := r.Read(data)
		if err != nil {
			return err
		}
		total += int64(cr)
		cw, err := outFile.Write(data[:cr])
		if err != nil {
			return err
		}
		if cw != cr {
			return fmt.Errorf("short wrote %d of %d for chunk %d", cw, cr, i)
		}
	}
	if total != l {
		return fmt.Errorf("received only %d of %d total bytes", total, l)
	}
	return nil
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

	err := c.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
