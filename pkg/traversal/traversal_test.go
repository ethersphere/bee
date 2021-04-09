// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traversal_test

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
)

var (
	simpleData       = []byte("hello test world") // fixed, 16 bytes
	defaultMediaType = "bzz-manifest-mantaray"
)

func generateSampleData(size int) (b []byte) {
	for {
		b = append(b, simpleData...)
		if len(b) >= size {
			break
		}
	}

	b = b[:size]

	return b
}

func TestTraversalBytes(t *testing.T) {
	traverseFn := func(traversalService traversal.Service) func(context.Context, swarm.Address, swarm.AddressIterFunc) error {
		return traversalService.TraverseBytesAddresses
	}

	testCases := []struct {
		dataSize            int
		expectedHashesCount int
		expectedHashes      []string
		ignoreDuplicateHash bool
	}{
		{
			dataSize:            len(simpleData),
			expectedHashesCount: 1,
			expectedHashes: []string{
				"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
			},
		},
		{
			dataSize:            swarm.ChunkSize,
			expectedHashesCount: 1,
			expectedHashes: []string{
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
		},
		{
			dataSize:            swarm.ChunkSize + 1,
			expectedHashesCount: 3,
			expectedHashes: []string{
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"dcbfb467950a28f8c5023b86d31de4ff3a337993e921ae623ae62c7190d60329", // bytes (1)
			},
		},
		{
			dataSize:            swarm.ChunkSize * 128,
			expectedHashesCount: 129,
			expectedHashes: []string{
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
			ignoreDuplicateHash: true,
		},
		{
			dataSize:            swarm.ChunkSize * 129,
			expectedHashesCount: 131,
			expectedHashes: []string{
				"150665dfbd81f80f5ba00a0caa2caa34f8b94e662e1dea769fe9ce7ea170bf25", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
			ignoreDuplicateHash: true,
		},
		{
			dataSize:            swarm.ChunkSize*129 - 1,
			expectedHashesCount: 131,
			expectedHashes: []string{
				"895610b2d795e7cc351a8336d46ba9ef37309d83267d272c6e257e46a78ecb7c", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"d18f0d81b832086376684558978cfe6773ed773178f84961c8b750fe72033a26", // bytes (4095)
			},
			ignoreDuplicateHash: true,
		},
		{
			dataSize:            swarm.ChunkSize*129 + 1,
			expectedHashesCount: 133,
			expectedHashes: []string{
				"023ee8b901702a999e9ef90ca2bc1c6db1daefb3f178b683a87b0fd613fd8e21", // root (joiner, chunk)
				"5060cfd2a34df0269b47201e1f202eb2a165d787a0c5043ceb29bb85b7567c61", // bytes (joiner [4096 * 128])
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
				"dcbfb467950a28f8c5023b86d31de4ff3a337993e921ae623ae62c7190d60329", // bytes (1)
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // bytes (joiner - [4096, 1])
			},
			ignoreDuplicateHash: true,
		},
	}

	for _, tc := range testCases {
		chunkCount := int(math.Ceil(float64(tc.dataSize) / swarm.ChunkSize))

		testName := fmt.Sprintf("%d-chunk-%d-bytes", chunkCount, tc.dataSize)

		t.Run(testName, func(t *testing.T) {
			var (
				mockStorer = mock.NewStorer()
			)

			ctx := context.Background()

			bytesData := generateSampleData(tc.dataSize)

			pipe := builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(bytesData), int64(len(bytesData)))
			if err != nil {
				t.Fatal(err)
			}

			traversalCheck(t, mockStorer, traverseFn, address, tc.expectedHashesCount, tc.expectedHashes, tc.ignoreDuplicateHash)
		})
	}

}

func TestTraversalFiles(t *testing.T) {
	traverseFn := func(traversalService traversal.Service) func(context.Context, swarm.Address, swarm.AddressIterFunc) error {
		return traversalService.TraverseAddresses
	}

	testCases := []struct {
		filesSize           int
		contentType         string
		filename            string
		expectedHashesCount int
		expectedHashes      []string
		ignoreDuplicateHash bool
	}{
		{
			filesSize:           len(simpleData),
			contentType:         "text/plain; charset=utf-8",
			filename:            "simple.txt",
			expectedHashesCount: 4,
			expectedHashes: []string{
				"ae16fb27474b41273c0deb355e4405d3cd0a6639f834285f97c75636c9e29df7", // root manifest
				"0cc878d32c96126d47f63fbe391114ee1438cd521146fc975dea1546d302b6c0", // mainifest root metadata
				"05e34f11a0967e8c09968b69c4f486f569ef58a31a197992e01304a1e59f8e75", // manifest file entry
				"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a", // bytes
			},
		},
		{
			filesSize:           swarm.ChunkSize,
			contentType:         "text/plain; charset=utf-8",
			expectedHashesCount: 6,
			expectedHashes: []string{
				"7e0a4b6cd542eb501f372438cbbbcd8a82c444740f00bdd54f4981f487bcf8b7", // root manifest
				"0cc878d32c96126d47f63fbe391114ee1438cd521146fc975dea1546d302b6c0", // manifest root metadata
				"3f538c3b5225111a79b3b1dbb5e269ca2115f2a7caf0e6925b773457cdef7be5", // manifest file entry (Edge)
				"2f09e41846a24201758db3535dc6c42d738180c8874d4d40d4f2924d0091521f", // manifest file entry (Edge)
				"b2662d17d51ce734695d993b44c0e2df34c3f50d5889e5bc3b8718838658e6b0", // manifest file entry (Value)
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes
			},
		},
		{
			filesSize:           swarm.ChunkSize + 1,
			contentType:         "text/plain; charset=utf-8",
			filename:            "simple.txt",
			expectedHashesCount: 6,
			expectedHashes: []string{
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

		testName := fmt.Sprintf("%d-chunk-%d-bytes", chunkCount, tc.filesSize)

		t.Run(testName, func(t *testing.T) {
			var (
				mockStorer = mock.NewStorer()
			)

			ctx := context.Background()

			bytesData := generateSampleData(tc.filesSize)

			pipe := builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			fr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(bytesData), int64(len(bytesData)))
			if err != nil {
				t.Fatal(err)
			}

			ls := loadsave.New(mockStorer, storage.ModePutRequest, false)
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

			reference, err := fManifest.Store(ctx)
			if err != nil {
				t.Fatal(err)
			}

			traversalCheck(t, mockStorer, traverseFn, reference, tc.expectedHashesCount, tc.expectedHashes, tc.ignoreDuplicateHash)
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
	traverseFn := func(traversalService traversal.Service) func(context.Context, swarm.Address, swarm.AddressIterFunc) error {
		return traversalService.TraverseManifestAddresses
	}

	testCases := []struct {
		files               []file
		manifestHashes      []string
		expectedHashesCount int
		ignoreDuplicateHash bool
	}{
		{
			files: []file{
				{
					size: len(simpleData),
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
			expectedHashesCount: 3,
		},
		{
			files: []file{
				{
					size: len(simpleData),
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
			expectedHashesCount: 8,
			ignoreDuplicateHash: true,
		},
	}

	for _, tc := range testCases {

		testName := fmt.Sprintf("%s-%d-files-%d-chunks", defaultMediaType, len(tc.files), tc.expectedHashesCount)

		t.Run(testName, func(t *testing.T) {
			var (
				mockStorer = mock.NewStorer()
			)

			expectedHashes := []string{}

			// add hashes for files
			for _, f := range tc.files {
				// add hash for each content
				expectedHashes = append(expectedHashes, f.chunks.content...)
			}

			// add hashes for manifest
			expectedHashes = append(expectedHashes, tc.manifestHashes...)

			ctx := context.Background()

			ls := loadsave.New(mockStorer, storage.ModePutRequest, false)
			dirManifest, err := manifest.NewMantarayManifest(ls, false)
			if err != nil {
				t.Fatal(err)
			}

			// add files to manifest
			for _, f := range tc.files {
				bytesData := generateSampleData(f.size)

				pipe := builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
				fr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(bytesData), int64(len(bytesData)))
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
			// save manifest
			manifestReference, err := dirManifest.Store(ctx)
			if err != nil {
				t.Fatal(err)
			}

			traversalCheck(t, mockStorer, traverseFn, manifestReference, tc.expectedHashesCount, expectedHashes, tc.ignoreDuplicateHash)
		})
	}

}

func traversalCheck(t *testing.T,
	storer storage.Storer,
	traverseFn func(traversalService traversal.Service) func(context.Context, swarm.Address, swarm.AddressIterFunc) error,
	reference swarm.Address,
	expectedHashesCount int,
	expectedHashes []string,
	ignoreDuplicateHash bool,
) {
	t.Helper()

	// sort input
	sort.Strings(expectedHashes)

	// traverse chunks
	traversalService := traversal.NewService(storer)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	foundAddressesCount := 0
	foundAddresses := make(map[string]struct{})
	var foundAddressesMu sync.Mutex

	err := traverseFn(traversalService)(
		ctx,
		reference,
		func(addr swarm.Address) error {
			foundAddressesMu.Lock()
			defer foundAddressesMu.Unlock()

			foundAddressesCount++
			if !ignoreDuplicateHash {
				if _, ok := foundAddresses[addr.String()]; ok {
					t.Fatalf("address found again: %s", addr.String())
				}
			}
			foundAddresses[addr.String()] = struct{}{}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}

	if expectedHashesCount != foundAddressesCount {
		t.Fatalf("expected to find %d addresses, got %d", expectedHashesCount, foundAddressesCount)
	}

	if !ignoreDuplicateHash {
		if len(expectedHashes) != len(foundAddresses) {
			t.Fatalf("expected to find %d addresses hashes, got %d", len(expectedHashes), len(foundAddresses))
		}
	}

	checkAddressFound := func(t *testing.T, foundAddresses map[string]struct{}, address string) {
		t.Helper()

		if _, ok := foundAddresses[address]; !ok {
			t.Fatalf("expected address %s not found", address)
		}
	}

	for _, createdAddress := range expectedHashes {
		checkAddressFound(t, foundAddresses, createdAddress)
	}
}
