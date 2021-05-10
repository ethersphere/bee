// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traversal

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	dataCorpus       = "hello test world" // 16 bytes.
	defaultMediaType = "bzz-manifest-mantaray"
)

func generateSample(size int) []byte {
	buf := make([]byte, size)
	for n := 0; n < size; {
		n += copy(buf[n:], dataCorpus)
	}
	return buf
}

// newAddressIterator is a convenient constructor for creating addressIterator.
func newAddressIterator(ignoreDuplicates bool) *addressIterator {
	return &addressIterator{
		seen:             make(map[string]bool),
		ignoreDuplicates: ignoreDuplicates,
	}
}

// addressIterator is a simple collector of statistics
// targeting swarm.AddressIterFunc execution.
type addressIterator struct {
	mu   sync.Mutex // mu guards cnt and seen fields.
	cnt  int
	seen map[string]bool
	// Settings.
	ignoreDuplicates bool
}

// Next matches the signature of swarm.AddressIterFunc needed in
// Traverser.Traverse method and collects statistics about it's execution.
func (i *addressIterator) Next(addr swarm.Address) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.cnt++
	if !i.ignoreDuplicates && i.seen[addr.String()] {
		return fmt.Errorf("duplicit address: %q", addr.String())
	}
	i.seen[addr.String()] = true
	return nil
}

var _ Traverser = (*Service)(nil)

func TestTraversalBytes(t *testing.T) {
	testCases := []struct {
		dataSize              int
		wantHashCount         int
		wantHashes            []string
		ignoreDuplicateHashes bool
	}{
		{
			dataSize:      len(dataCorpus),
			wantHashCount: 1,
			wantHashes: []string{
				"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
			},
		},
		{
			dataSize:      swarm.ChunkSize,
			wantHashCount: 1,
			wantHashes: []string{
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
		},
		{
			dataSize:      swarm.ChunkSize + 1,
			wantHashCount: 3,
			wantHashes: []string{
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"dcbfb467950a28f8c5023b86d31de4ff3a337993e921ae623ae62c7190d60329", // bytes (1)
			},
		},
		{
			dataSize:      swarm.ChunkSize * 128,
			wantHashCount: 129,
			wantHashes: []string{
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
			ignoreDuplicateHashes: true,
		},
		{
			dataSize:      swarm.ChunkSize * 129,
			wantHashCount: 131,
			wantHashes: []string{
				"150665dfbd81f80f5ba00a0caa2caa34f8b94e662e1dea769fe9ce7ea170bf25", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
			ignoreDuplicateHashes: true,
		},
		{
			dataSize:      swarm.ChunkSize*129 - 1,
			wantHashCount: 131,
			wantHashes: []string{
				"895610b2d795e7cc351a8336d46ba9ef37309d83267d272c6e257e46a78ecb7c", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"d18f0d81b832086376684558978cfe6773ed773178f84961c8b750fe72033a26", // bytes (4095)
			},
			ignoreDuplicateHashes: true,
		},
		{
			dataSize:      swarm.ChunkSize*129 + 1,
			wantHashCount: 133,
			wantHashes: []string{
				"023ee8b901702a999e9ef90ca2bc1c6db1daefb3f178b683a87b0fd613fd8e21", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner [4096 * 128])
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"dcbfb467950a28f8c5023b86d31de4ff3a337993e921ae623ae62c7190d60329", // bytes (1)
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // bytes (joiner - [4096, 1])
			},
			ignoreDuplicateHashes: true,
		},
	}

	for _, tc := range testCases {
		chunkCount := int(math.Ceil(float64(tc.dataSize) / swarm.ChunkSize))
		t.Run(fmt.Sprintf("%d-chunk-%d-bytes", chunkCount, tc.dataSize), func(t *testing.T) {
			var (
				data       = generateSample(tc.dataSize)
				iter       = newAddressIterator(tc.ignoreDuplicateHashes)
				storerMock = mock.NewStorer()
			)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pipe := builder.NewPipelineBuilder(ctx, storerMock, storage.ModePutUpload, false)
			address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}

			err = NewService(storerMock).Traverse(ctx, address, iter.Next)
			if err != nil {
				t.Fatal(err)
			}

			haveCnt, wantCnt := tc.wantHashCount, iter.cnt
			if !tc.ignoreDuplicateHashes {
				haveCnt, wantCnt = len(iter.seen), len(tc.wantHashes)
			}
			if haveCnt != wantCnt {
				t.Fatalf("hash count mismatch: have %d; want %d", haveCnt, wantCnt)
			}

			for _, hash := range tc.wantHashes {
				if !iter.seen[hash] {
					t.Fatalf("hash check: want %q; have none", hash)
				}
			}
		})
	}
}

func TestTraversalFiles(t *testing.T) {
	testCases := []struct {
		filesSize             int
		contentType           string
		filename              string
		wantHashCount         int
		wantHashes            []string
		ignoreDuplicateHashes bool
	}{
		{
			filesSize:     len(dataCorpus),
			contentType:   "text/plain; charset=utf-8",
			filename:      "simple.txt",
			wantHashCount: 4,
			wantHashes: []string{
				"ae16fb27474b41273c0deb355e4405d3cd0a6639f834285f97c75636c9e29df7", // root manifest
				"0cc878d32c96126d47f63fbe391114ee1438cd521146fc975dea1546d302b6c0", // mainifest root metadata
				"05e34f11a0967e8c09968b69c4f486f569ef58a31a197992e01304a1e59f8e75", // manifest file entry
				"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a", // bytes
			},
		},
		{
			filesSize:     swarm.ChunkSize,
			contentType:   "text/plain; charset=utf-8",
			wantHashCount: 6,
			wantHashes: []string{
				"7e0a4b6cd542eb501f372438cbbbcd8a82c444740f00bdd54f4981f487bcf8b7", // root manifest
				"0cc878d32c96126d47f63fbe391114ee1438cd521146fc975dea1546d302b6c0", // manifest root metadata
				"3f538c3b5225111a79b3b1dbb5e269ca2115f2a7caf0e6925b773457cdef7be5", // manifest file entry (Edge)
				"2f09e41846a24201758db3535dc6c42d738180c8874d4d40d4f2924d0091521f", // manifest file entry (Edge)
				"b2662d17d51ce734695d993b44c0e2df34c3f50d5889e5bc3b8718838658e6b0", // manifest file entry (Value)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes
			},
		},
		{
			filesSize:     swarm.ChunkSize + 1,
			contentType:   "text/plain; charset=utf-8",
			filename:      "simple.txt",
			wantHashCount: 6,
			wantHashes: []string{
				"ea58761906f98bd88204efbbab5c690329af02548afec37d7a556a47ca78ac62", // manifest root
				"0cc878d32c96126d47f63fbe391114ee1438cd521146fc975dea1546d302b6c0", // manifest root metadata
				"85617df0249a12649b56d09cf7f21e8642627b4fb9c0c9e03e2d25340cf60499", // manifest file entry
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // manifest file entry
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"dcbfb467950a28f8c5023b86d31de4ff3a337993e921ae623ae62c7190d60329", // bytes (1)
			},
		},
	}

	for _, tc := range testCases {
		chunkCount := int(math.Ceil(float64(tc.filesSize) / swarm.ChunkSize))
		t.Run(fmt.Sprintf("%d-chunk-%d-bytes", chunkCount, tc.filesSize), func(t *testing.T) {
			var (
				data       = generateSample(tc.filesSize)
				iter       = newAddressIterator(tc.ignoreDuplicateHashes)
				storerMock = mock.NewStorer()
			)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			pipe := builder.NewPipelineBuilder(ctx, storerMock, storage.ModePutUpload, false)
			fr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}

			ls := loadsave.New(storerMock, storage.ModePutRequest, false)
			fManifest, err := manifest.NewDefaultManifest(ls, false)
			if err != nil {
				t.Fatal(err)
			}
			filename := tc.filename
			if filename == "" {
				filename = fr.String()
			}

			rootMtdt := map[string]string{
				manifest.WebsiteIndexDocumentSuffixKey: filename,
			}
			err = fManifest.Add(ctx, "/", manifest.NewEntry(swarm.ZeroAddress, rootMtdt))
			if err != nil {
				t.Fatal(err)
			}

			fileMtdt := map[string]string{
				manifest.EntryMetadataFilenameKey:    filename,
				manifest.EntryMetadataContentTypeKey: tc.contentType,
			}
			err = fManifest.Add(ctx, filename, manifest.NewEntry(fr, fileMtdt))
			if err != nil {
				t.Fatal(err)
			}

			address, err := fManifest.Store(ctx)
			if err != nil {
				t.Fatal(err)
			}

			err = NewService(storerMock).Traverse(ctx, address, iter.Next)
			if err != nil {
				t.Fatal(err)
			}

			haveCnt, wantCnt := tc.wantHashCount, iter.cnt
			if !tc.ignoreDuplicateHashes {
				haveCnt, wantCnt = len(iter.seen), len(tc.wantHashes)
			}
			if haveCnt != wantCnt {
				t.Fatalf("hash count mismatch: have %d; want %d", haveCnt, wantCnt)
			}

			for _, hash := range tc.wantHashes {
				if !iter.seen[hash] {
					t.Fatalf("hash check: want %q; have none", hash)
				}
			}
		})
	}
}

type file struct {
	size   int
	dir    string
	name   string
	chunks fileChunks
}

type fileChunks struct {
	content []string
}

func TestTraversalManifest(t *testing.T) {
	testCases := []struct {
		files                 []file
		manifestHashes        []string
		wantHashCount         int
		ignoreDuplicateHashes bool
	}{
		{
			files: []file{
				{
					size: len(dataCorpus),
					dir:  "",
					name: "hello.txt",
					chunks: fileChunks{
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
			},
			manifestHashes: []string{
				// NOTE: references will be fixed, due to custom obfuscation key function
				"f81ac8ceb2db7e55b718eca35f05233dc523022e36e11f934dbfd5f0cafde198", // root
				"05e34f11a0967e8c09968b69c4f486f569ef58a31a197992e01304a1e59f8e75", // metadata
			},
			wantHashCount: 3,
		},
		{
			files: []file{
				{
					size: len(dataCorpus),
					dir:  "",
					name: "hello.txt",
					chunks: fileChunks{
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
				{
					size: swarm.ChunkSize,
					dir:  "",
					name: "data/1.txt",
					chunks: fileChunks{
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
				{
					size: swarm.ChunkSize,
					dir:  "",
					name: "data/2.txt",
					chunks: fileChunks{
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
			},
			manifestHashes: []string{
				// NOTE: references will be fixed, due to custom obfuscation key function
				"d182df1cb214167d085256fafa657f38a191efe51af16834f6288ef23416fd25", // root
				"05e34f11a0967e8c09968b69c4f486f569ef58a31a197992e01304a1e59f8e75", // manifest entry
				"7e6bc53ca11bff459f77892563d04e09b440c63ce2f7d5fe8a8b0f0ba9eeefcf", // manifest entry (Edge PathSeparator)
				"b2662d17d51ce734695d993b44c0e2df34c3f50d5889e5bc3b8718838658e6b0", // manifest file entry (1.txt)
				"b2662d17d51ce734695d993b44c0e2df34c3f50d5889e5bc3b8718838658e6b0", // manifest file entry (2.txt)
			},
			wantHashCount:         8,
			ignoreDuplicateHashes: true,
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%d-files-%d-chunks", defaultMediaType, len(tc.files), tc.wantHashCount), func(t *testing.T) {
			var (
				storerMock = mock.NewStorer()
				iter       = newAddressIterator(tc.ignoreDuplicateHashes)
			)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var wantHashes []string
			for _, f := range tc.files {
				wantHashes = append(wantHashes, f.chunks.content...)
			}
			wantHashes = append(wantHashes, tc.manifestHashes...)

			ls := loadsave.New(storerMock, storage.ModePutRequest, false)
			dirManifest, err := manifest.NewMantarayManifest(ls, false)
			if err != nil {
				t.Fatal(err)
			}

			for _, f := range tc.files {
				data := generateSample(f.size)

				pipe := builder.NewPipelineBuilder(ctx, storerMock, storage.ModePutUpload, false)
				fr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}

				fileName := f.name
				if fileName == "" {
					fileName = fr.String()
				}
				filePath := path.Join(f.dir, fileName)

				err = dirManifest.Add(ctx, filePath, manifest.NewEntry(fr, nil))
				if err != nil {
					t.Fatal(err)
				}
			}
			address, err := dirManifest.Store(ctx)
			if err != nil {
				t.Fatal(err)
			}

			err = NewService(storerMock).Traverse(ctx, address, iter.Next)
			if err != nil {
				t.Fatal(err)
			}

			haveCnt, wantCnt := tc.wantHashCount, iter.cnt
			if !tc.ignoreDuplicateHashes {
				haveCnt, wantCnt = len(iter.seen), len(wantHashes)
			}
			if haveCnt != wantCnt {
				t.Fatalf("hash count mismatch: have %d; want %d", haveCnt, wantCnt)
			}

			for _, hash := range wantHashes {
				if !iter.seen[hash] {
					t.Fatalf("hash check: want %q; have none", hash)
				}
			}
		})
	}
}
