// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"archive/tar"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	// filename in tar archive that holds the information
	// about exported data format version
	exportVersionFilename = ".swarm-export-version"
	// current export format version
	currentExportVersion = "2"
)

// Export writes a tar structured data to the writer of
// all chunks in the retrieval data index. It returns the
// number of chunks exported.
func (db *DB) Export(w io.Writer) (count int64, err error) {
	tw := tar.NewWriter(w)
	defer tw.Close()

	if err := tw.WriteHeader(&tar.Header{
		Name: exportVersionFilename,
		Mode: 0644,
		Size: int64(len(currentExportVersion)),
	}); err != nil {
		return 0, err
	}
	if _, err := tw.Write([]byte(currentExportVersion)); err != nil {
		return 0, err
	}

	err = db.retrievalDataIndex.Iterate(func(item shed.Item) (stop bool, err error) {

		hdr := &tar.Header{
			Name: hex.EncodeToString(item.Address),
			Mode: 0644,
			Size: int64(postage.StampSize + len(item.Data)),
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return false, err
		}
		if _, err := tw.Write(item.BatchID); err != nil {
			return false, err
		}
		if _, err := tw.Write(item.Sig); err != nil {
			return false, err
		}
		if _, err := tw.Write(item.Data); err != nil {
			return false, err
		}
		count++
		return false, nil
	}, nil)

	return count, err
}

// Import reads a tar structured data from the reader and
// stores chunks in the database. It returns the number of
// chunks imported.
func (db *DB) Import(ctx context.Context, r io.Reader) (count int64, err error) {
	tr := tar.NewReader(r)

	errC := make(chan error)
	doneC := make(chan struct{})
	tokenPool := make(chan struct{}, 100)
	var wg sync.WaitGroup
	go func() {
		var (
			firstFile = true

			// if exportVersionFilename file is not present
			// assume current version
			version = currentExportVersion
		)
		for {
			hdr, err := tr.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				select {
				case errC <- err:
				case <-ctx.Done():
				}
			}
			if firstFile {
				firstFile = false
				if hdr.Name == exportVersionFilename {
					data, err := ioutil.ReadAll(tr)
					if err != nil {
						select {
						case errC <- err:
						case <-ctx.Done():
						}
					}
					version = string(data)
					continue
				}
			}

			if len(hdr.Name) != 64 {
				db.logger.Warningf("localstore export: ignoring non-chunk file: %s", hdr.Name)
				continue
			}

			keybytes, err := hex.DecodeString(hdr.Name)
			if err != nil {
				db.logger.Warningf("localstore export: ignoring invalid chunk file %s: %v", hdr.Name, err)
				continue
			}

			rawdata, err := ioutil.ReadAll(tr)
			if err != nil {
				select {
				case errC <- err:
				case <-ctx.Done():
				}
			}
			stamp := new(postage.Stamp)
			err = stamp.UnmarshalBinary(rawdata[:postage.StampSize])
			if err != nil {
				select {
				case errC <- err:
				case <-ctx.Done():
				}
			}
			data := rawdata[postage.StampSize:]
			key := swarm.NewAddress(keybytes)

			var ch swarm.Chunk
			switch version {
			case currentExportVersion:
				ch = swarm.NewChunk(key, data).WithStamp(stamp)
			default:
				select {
				case errC <- fmt.Errorf("unsupported export data version %q", version):
				case <-ctx.Done():
				}
			}
			tokenPool <- struct{}{}
			wg.Add(1)

			go func() {
				_, err := db.Put(ctx, storage.ModePutUpload, ch)
				select {
				case errC <- err:
				case <-ctx.Done():
					wg.Done()
					<-tokenPool
				default:
					_, err := db.Put(ctx, storage.ModePutUpload, ch)
					if err != nil {
						errC <- err
					}
					wg.Done()
					<-tokenPool
				}
			}()

			count++
		}
		wg.Wait()
		close(doneC)
	}()

	// wait for all chunks to be stored
	for {
		select {
		case err := <-errC:
			if err != nil {
				return count, err
			}
		case <-ctx.Done():
			return count, ctx.Err()
		default:
			select {
			case <-doneC:
				return count, nil
			default:
			}
		}
	}
}
