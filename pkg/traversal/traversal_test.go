// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package traversal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"mime"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/collection/entry"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
)

var (
	simpleData = []byte("hello test world") // fixed, 16 bytes
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
		return traversalService.TraverseFileAddresses
	}

	testCases := []struct {
		filesSize           int
		contentType         string
		expectedHashesCount int
		expectedHashes      []string
		ignoreDuplicateHash bool
	}{
		{
			filesSize:           len(simpleData),
			contentType:         "text/plain; charset=utf-8",
			expectedHashesCount: 3,
			expectedHashes: []string{
				"06e50210b6bcebca15cfc8bc9ee3aa51ad8fa9cac41340f9f6396ada74fec78f", // root
				"999a9f2e1fd29a6691a3b8e437cbb36e34a1f67decc973dfc70928d1e7de3c3b", // metadata
				"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a", // bytes
			},
		},
		{
			filesSize:           swarm.ChunkSize,
			contentType:         "text/plain; charset=utf-8",
			expectedHashesCount: 3,
			expectedHashes: []string{
				"29ae87fda18bee4255ef19faabe901e2cf9c1c5c4648083383255670492e814e", // root
				"e7d4d4a897cd69f5759621044402e40a3d5c903cf1e225864eef5d1f77d97680", // metadata
				"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
			},
		},
		{
			filesSize:           swarm.ChunkSize + 1,
			contentType:         "text/plain; charset=utf-8",
			expectedHashesCount: 5,
			expectedHashes: []string{
				"aa4a46bfbdff91c8db555edcfa4ba18371a083fdec67120db58d7ef177815ff0", // root
				"be1f048819e744886803fbe44cf16205949b196640665077bfcacf68c323aa49", // metadata
				"a1c4483d15167aeb406017942c9625464574cf70bf7e42f237094acbccdb6834", // bytes (joiner)
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

			fileName := fr.String()

			m := entry.NewMetadata(fileName)
			m.MimeType = tc.contentType
			metadataBytes, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}

			pipe = builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			mr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(metadataBytes), int64(len(metadataBytes)))
			if err != nil {
				t.Fatal(err)
			}

			entrie := entry.New(fr, mr)
			fileEntryBytes, err := entrie.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			pipe = builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			reference, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)))
			if err != nil {
				t.Fatal(err)
			}

			traversalCheck(t, mockStorer, traverseFn, reference, tc.expectedHashesCount, tc.expectedHashes, tc.ignoreDuplicateHash)
		})
	}

}

type file struct {
	size      int
	dir       string
	name      string
	reference string
	chunks    fileChunks
}

type fileChunks struct {
	metadata string
	content  []string
}

func TestTraversalManifest(t *testing.T) {
	traverseFn := func(traversalService traversal.Service) func(context.Context, swarm.Address, swarm.AddressIterFunc) error {
		return traversalService.TraverseManifestAddresses
	}

	testCases := []struct {
		manifestType        string
		files               []file
		manifestHashes      []string
		expectedHashesCount int
		ignoreDuplicateHash bool
	}{
		{
			manifestType: manifest.ManifestSimpleContentType,
			files: []file{
				{
					size:      len(simpleData),
					dir:       "",
					name:      "hello.txt",
					reference: "a7c9250614bd2d2529e7bee2e2d0df295661b7185465193dc3b54ffea30c4702",
					chunks: fileChunks{
						metadata: "af2f73f800821b8ca7f5d2c33d0ba6018734d809389a47993c621cc62245d9e0",
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
			},
			manifestHashes: []string{
				"864984d3b0a0401123325ffac8ce696f3eb67ea9ba290a66e8d4e7ddb41fd1dc", // root
				"90cca4ac6ec25d8fdae297f65dfa389abd2db77f1b44a623d9fcb96802a935a7", // metadata
				"3665a0de7b2a63ba80fd3bb6f7c2d75b633ee4a297a0d7442cecd89c3553a4d2", // bytes
			},
			expectedHashesCount: 6,
		},
		{
			manifestType: manifest.ManifestSimpleContentType,
			files: []file{
				{
					size:      len(simpleData),
					dir:       "",
					name:      "hello.txt",
					reference: "a7c9250614bd2d2529e7bee2e2d0df295661b7185465193dc3b54ffea30c4702",
					chunks: fileChunks{
						metadata: "af2f73f800821b8ca7f5d2c33d0ba6018734d809389a47993c621cc62245d9e0",
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
				{
					size:      swarm.ChunkSize,
					dir:       "",
					name:      "data/1.txt",
					reference: "5241139a93e4c8735b62414c4a3be8d10e83c6644af320f8892cbac0bc869cab",
					chunks: fileChunks{
						metadata: "ec35ef758093abaeaabc3956c8eeb9739cf6e6168ce44ae912b9b4777b0e9420",
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
				{
					size:      swarm.ChunkSize,
					dir:       "",
					name:      "data/2.txt",
					reference: "940d67638f577ad36701b7ed380ed8e1c4c14e6bb6e19c6a74b0d5ac7cb0fb55",
					chunks: fileChunks{
						metadata: "a05586fb3c4625e21377ce2043c362835d3eb95bd9970d84db414a0f6164f822",
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
			},
			manifestHashes: []string{
				"d2c4586f8791058153464064aa9b90059ad8ab9afe068df37d97f5711a0a197f", // root
				"39745d382da0c21042290c59d43840a5685f461bd7da49c36a120136f49869cb", // metadata
				"dc763a70a578970c001cb9c59c90615d3e5c19eb4147cc45757481e32bf72ec7", // bytes
			},
			expectedHashesCount: 12,
			ignoreDuplicateHash: true,
		},
		{
			manifestType: manifest.ManifestMantarayContentType,
			files: []file{
				{
					size:      len(simpleData),
					dir:       "",
					name:      "hello.txt",
					reference: "a7c9250614bd2d2529e7bee2e2d0df295661b7185465193dc3b54ffea30c4702",
					chunks: fileChunks{
						metadata: "af2f73f800821b8ca7f5d2c33d0ba6018734d809389a47993c621cc62245d9e0",
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
			},
			manifestHashes: []string{
				// NOTE: references will be fixed, due to custom obfuscation key function
				"596c29bd00b241cb38aba10ca7005bf124baed90b613c2ff11ee891165a487fd", // root
				"70501ac2caed16fc5f929977172a631ac540a5efd567cf1447bf7ee4aae4eb9f", // metadata
				"486914d1449e482ff248268e99c5d7d2772281f033c07f2f74aa4cc1ce3a8fe0", // bytes - root node
				"3d6a9e4eec6ebaf6ca6c6412dae6a23c76bc0c0672d259d98562368915d16b88", // bytes - node [h]
			},
			expectedHashesCount: 7,
		},
		{
			manifestType: manifest.ManifestMantarayContentType,
			files: []file{
				{
					size:      len(simpleData),
					dir:       "",
					name:      "hello.txt",
					reference: "a7c9250614bd2d2529e7bee2e2d0df295661b7185465193dc3b54ffea30c4702",
					chunks: fileChunks{
						metadata: "af2f73f800821b8ca7f5d2c33d0ba6018734d809389a47993c621cc62245d9e0",
						content: []string{
							"e94a5aadf259f008b7d5039420c65d692901846523f503d97d24e2f077786d9a",
						},
					},
				},
				{
					size:      swarm.ChunkSize,
					dir:       "",
					name:      "data/1.txt",
					reference: "5241139a93e4c8735b62414c4a3be8d10e83c6644af320f8892cbac0bc869cab",
					chunks: fileChunks{
						metadata: "ec35ef758093abaeaabc3956c8eeb9739cf6e6168ce44ae912b9b4777b0e9420",
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
				{
					size:      swarm.ChunkSize,
					dir:       "",
					name:      "data/2.txt",
					reference: "940d67638f577ad36701b7ed380ed8e1c4c14e6bb6e19c6a74b0d5ac7cb0fb55",
					chunks: fileChunks{
						metadata: "a05586fb3c4625e21377ce2043c362835d3eb95bd9970d84db414a0f6164f822",
						content: []string{
							"f833c17be12d68aec95eca7f9d993f7d7aaa7a9c282eb2c3d79ab26a5aeaf384", // bytes (4096)
						},
					},
				},
			},
			manifestHashes: []string{
				// NOTE: references will be fixed, due to custom obfuscation key function
				"10a70b3a0102b94e909d08b91b98a2d8ca22c762ad7286d5451de2dd6432c218", // root
				"fb2c46942a3b2148e856d778731de9c173a26bec027aa27897f32e423eb14458", // metadata
				"39caaed3c9e42ea3ad9a374d37181e21c9a686367e0ae42d66c20465538d9789", // bytes - root node
				"735aee067bdc02e1c1e8e88eea8b5b0535bfc9d0d36bf3a4d6fbac94a03bc233", // bytes - node [d]
				"3d6a9e4eec6ebaf6ca6c6412dae6a23c76bc0c0672d259d98562368915d16b88", // bytes - node [h]
				"ddb31ae6a74caf5df03e5d8bf6056e589229b4cae3087433db64a4768923f73b", // bytes - node [d]/[2]
				"281dc7467f647abbfbaaf259a95ab60df8bf76ec3fbc525bfbca794d6360fa46", // bytes - node [d]/[1]
			},
			expectedHashesCount: 16,
			ignoreDuplicateHash: true,
		},
	}

	for _, tc := range testCases {
		mediatype, _, err := mime.ParseMediaType(tc.manifestType)
		if err != nil {
			t.Fatal(err)
		}

		mediatype = strings.Split(mediatype, "/")[1]
		mediatype = strings.Split(mediatype, "+")[0]

		testName := fmt.Sprintf("%s-%d-files-%d-chunks", mediatype, len(tc.files), tc.expectedHashesCount)

		t.Run(testName, func(t *testing.T) {
			var (
				mockStorer = mock.NewStorer()
			)

			expectedHashes := []string{}

			// add hashes for files
			for _, f := range tc.files {
				expectedHashes = append(expectedHashes, f.reference, f.chunks.metadata)
				// add hash for each content
				expectedHashes = append(expectedHashes, f.chunks.content...)
			}

			// add hashes for manifest
			expectedHashes = append(expectedHashes, tc.manifestHashes...)

			ctx := context.Background()

			var dirManifest manifest.Interface
			ls := loadsave.New(mockStorer, storage.ModePutRequest, false)
			switch tc.manifestType {
			case manifest.ManifestSimpleContentType:
				dirManifest, err = manifest.NewSimpleManifest(ls)
				if err != nil {
					t.Fatal(err)
				}
			case manifest.ManifestMantarayContentType:
				dirManifest, err = manifest.NewMantarayManifest(ls, false)
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("manifest: invalid type: %s", tc.manifestType)
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

				m := entry.NewMetadata(fileName)
				metadataBytes, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}

				pipe = builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
				mr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(metadataBytes), int64(len(metadataBytes)))
				if err != nil {
					t.Fatal(err)
				}

				entrie := entry.New(fr, mr)
				fileEntryBytes, err := entrie.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}

				pipe = builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
				reference, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)))
				if err != nil {
					t.Fatal(err)
				}

				filePath := path.Join(f.dir, fileName)

				err = dirManifest.Add(ctx, filePath, manifest.NewEntry(reference, nil))
				if err != nil {
					t.Fatal(err)
				}
			}

			// save manifest
			manifestBytesReference, err := dirManifest.Store(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// store the manifest metadata and get its reference
			m := entry.NewMetadata(manifestBytesReference.String())
			m.MimeType = dirManifest.Type()
			metadataBytes, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}

			pipe := builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			mr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(metadataBytes), int64(len(metadataBytes)))
			if err != nil {
				t.Fatal(err)
			}

			// now join both references (fr, mr) to create an entry and store it
			e := entry.New(manifestBytesReference, mr)
			fileEntryBytes, err := e.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}

			pipe = builder.NewPipelineBuilder(ctx, mockStorer, storage.ModePutUpload, false)
			manifestFileReference, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(fileEntryBytes), int64(len(fileEntryBytes)))
			if err != nil {
				t.Fatal(err)
			}

			traversalCheck(t, mockStorer, traverseFn, manifestFileReference, tc.expectedHashesCount, expectedHashes, tc.ignoreDuplicateHash)
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
