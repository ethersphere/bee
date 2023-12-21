// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/manifest"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	mockstorer "github.com/ethersphere/bee/pkg/storer/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/util/ioutil/pseudorand"
)

// nolint:paralleltest,tparallel
func TestBzzUploadWithRedundancy(t *testing.T) {
	fileUploadResource := "/bzz"
	fileDownloadResource := func(addr string) string { return "/bzz/" + addr }
	fetchTimeout := 200 * time.Millisecond
	storerMock := mockstorer.NewWithChunkStore(mockstorer.NewForgettingStore(inmemchunkstore.New(), 2, fetchTimeout))
	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: storerMock,
		Logger: log.NewLogger("stdout", log.WithVerbosity(log.VerbosityDebug), log.WithCallerFunc()),
		// Logger: log.Noop ,
		Post: mockpost.New(mockpost.WithAcceptAll()),
	})

	type testCase struct {
		name       string
		redundancy string
		size       int
		encrypt    string
	}
	var tcs []testCase
	for _, encrypt := range []string{"false", "true"} {
		for _, level := range []string{"0", "1", "2", "3", "4"} {
			for _, size := range []int{1, 42, 420, 4095, 4096, 4200, 128*4096 + 4095} {
				tcs = append(tcs, testCase{
					name:       fmt.Sprintf("level-%s-encrypt-%s-size-%d", level, encrypt, size),
					redundancy: level,
					size:       size,
					encrypt:    encrypt,
				})
			}
		}
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			seed, err := pseudorand.NewSeed()
			if err != nil {
				t.Fatal(err)
			}
			reader := pseudorand.NewReader(seed, tc.size)

			var refResponse api.BzzUploadResponse
			jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource,
				http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "True"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestBody(reader),
				jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, tc.encrypt),
				jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, tc.redundancy),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
				jsonhttptest.WithUnmarshalJSONResponse(&refResponse),
			)

			t.Run("download without redundancy should fail", func(t *testing.T) {
				t.Skip("no")
				req, err := http.NewRequest("GET", fileDownloadResource(refResponse.Reference.String()), nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set(api.SwarmRedundancyStrategyHeader, "0")
				req.Header.Set(api.SwarmRedundancyFallbackModeHeader, "false")
				req.Header.Set(api.SwarmChunkRetrievalTimeoutHeader, fetchTimeout.String())

				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusNotFound {
					t.Fatalf("expected status %d; got %d", http.StatusNotFound, resp.StatusCode)
				}
			})

			t.Run("download with redundancy should succeed", func(t *testing.T) {
				// t.Skip("no")
				req, err := http.NewRequest("GET", fileDownloadResource(refResponse.Reference.String()), nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set(api.SwarmRedundancyStrategyHeader, "0")
				req.Header.Set(api.SwarmRedundancyFallbackModeHeader, "true")
				req.Header.Set(api.SwarmChunkRetrievalTimeoutHeader, fetchTimeout.String())

				resp, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("expected status %d; got %d", http.StatusOK, resp.StatusCode)
				}

				if !reader.Equal(resp.Body) {
					t.Fatalf("content mismatch")
				}
			})
		})
	}
}

func TestBzzFiles(t *testing.T) {
	t.Parallel()

	var (
		fileUploadResource   = "/bzz"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		simpleData           = []byte("this is a simple text")
		storerMock           = mockstorer.New()
		logger               = log.Noop
		client, _, _, _      = newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	t.Run("tar-file-upload", func(t *testing.T) {
		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
			{
				data: []byte("image 1"),
				name: "1.png",
				dir:  "img",
				header: http.Header{
					api.ContentTypeHeader: {"image/png"},
				},
			},
			{
				data: []byte("image 2"),
				name: "2.png",
				dir:  "img",
				header: http.Header{
					api.ContentTypeHeader: {"image/png"},
				},
			},
		})
		address := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
		rcvdHeader := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: address,
			}),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
		)

		isTagFoundInResponse(t, rcvdHeader, nil)

		has, err := storerMock.ChunkStore().Has(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk address: have none; want one")
		}

		refs, err := storerMock.Pins()
		if err != nil {
			t.Fatal("unable to get pinned references")
		}
		if have, want := len(refs), 0; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
	})

	t.Run("tar-file-upload-with-pinning", func(t *testing.T) {
		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
			{
				data: []byte("image 1"),
				name: "1.png",
				dir:  "img",
				header: http.Header{
					api.ContentTypeHeader: {"image/png"},
				},
			},
			{
				data: []byte("image 2"),
				name: "2.png",
				dir:  "img",
				header: http.Header{
					api.ContentTypeHeader: {"image/png"},
				},
			},
		})
		reference := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
		rcvdHeader := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: reference,
			}),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
		)

		isTagFoundInResponse(t, rcvdHeader, nil)

		has, err := storerMock.ChunkStore().Has(context.Background(), reference)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk reference: have none; want one")
		}

		refs, err := storerMock.Pins()
		if err != nil {
			t.Fatal(err)
		}
		if have, want := len(refs), 1; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
		if have, want := refs[0], reference; !have.Equal(want) {
			t.Fatalf("root pin reference mismatch: have %q; want %q", have, want)
		}
	})

	t.Run("encrypt-decrypt", func(t *testing.T) {
		fileName := "my-pictures.jpeg"

		var resp api.BzzUploadResponse
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, "True"),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(resp.Reference.String()), http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(simpleData)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
			jsonhttptest.WithExpectedResponse(simpleData),
		)
	})

	t.Run("redundancy", func(t *testing.T) {
		fileName := "my-pictures.jpeg"

		var resp api.BzzUploadResponse
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, "True"),
			jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, "4"),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(resp.Reference.String()), http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(simpleData)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
			jsonhttptest.WithExpectedResponse(simpleData),
		)
	})

	t.Run("filter out filename path", func(t *testing.T) {
		fileName := "my-pictures.jpeg"
		fileNameWithPath := "../../" + fileName

		var resp api.BzzUploadResponse

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileNameWithPath, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		rootHash := resp.Reference.String()
		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(simpleData)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
			jsonhttptest.WithExpectedResponse(simpleData),
		)
	})

	t.Run("check-content-type-detection", func(t *testing.T) {
		fileName := "my-pictures.jpeg"
		rootHash := "4f9146b3813ccbd7ce45a18be23763d7e436ab7a3982ef39961c6f3cd4da1dcf"

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedResponse(simpleData),
			jsonhttptest.WithExpectedContentLength(len(simpleData)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "image/jpeg; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("upload-then-download-and-check-data", func(t *testing.T) {
		fileName := "sample.html"
		rootHash := "36e6c1bbdfee6ac21485d5f970479fd1df458d36df9ef4e8179708ed46da557f"
		sampleHtml := `<!DOCTYPE html>
		<html>
		<body>

		<h1>My First Heading</h1>

		<p>My first paragraph.</p>

		</body>
		</html>`

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(sampleHtml)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", rootHash)),
		)

		// try to fetch the same file and check the data
		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedResponse([]byte(sampleHtml)),
			jsonhttptest.WithExpectedContentLength(len(sampleHtml)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("upload-then-download-with-targets", func(t *testing.T) {
		fileName := "simple_file.txt"
		rootHash := "65148cd89b58e91616773f5acea433f7b5a6274f2259e25f4893a332b74a7e28"

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
		)
	})

}

// TestRangeRequests validates that all endpoints are serving content with
// respect to HTTP Range headers.
func TestBzzFilesRangeRequests(t *testing.T) {
	t.Parallel()

	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vivamus dignissim tincidunt orci id aliquam. Praesent eget turpis in lectus semper consectetur et ut nibh. Nam rhoncus, augue sit amet sollicitudin lacinia, turpis tortor molestie urna, at mattis sem sapien sit amet augue. In bibendum ex vel odio dignissim interdum. Quisque hendrerit sapien et porta condimentum. Vestibulum efficitur mauris tellus, eget vestibulum sapien vulputate ac. Proin et vulputate sapien. Duis tincidunt mauris vulputate porta venenatis. Sed dictum aliquet urna, sit amet fermentum velit pellentesque vitae. Nam sed nisi ultrices, volutpat quam et, malesuada sapien. Nunc gravida non orci at rhoncus. Sed vitae dui accumsan, venenatis lectus et, mattis tellus. Proin sed mauris eu mi congue lacinia.")

	uploads := []struct {
		name             string
		uploadEndpoint   string
		downloadEndpoint string
		filepath         string
		reader           io.Reader
		contentType      string
	}{
		{
			name:             "bytes",
			uploadEndpoint:   "/bytes",
			downloadEndpoint: "/bytes",
			reader:           bytes.NewReader(data),
			contentType:      "text/plain; charset=utf-8",
		},
		{
			name:             "file",
			uploadEndpoint:   "/bzz",
			downloadEndpoint: "/bzz",
			reader:           bytes.NewReader(data),
			contentType:      "text/plain; charset=utf-8",
		},
		{
			name:             "dir",
			uploadEndpoint:   "/bzz",
			downloadEndpoint: "/bzz",
			filepath:         "ipsum/lorem.txt",
			reader: tarFiles(t, []f{
				{
					data: data,
					name: "lorem.txt",
					dir:  "ipsum",
					header: http.Header{
						api.ContentTypeHeader: {"text/plain; charset=utf-8"},
					},
				},
			}),
			contentType: api.ContentTypeTar,
		},
	}

	ranges := []struct {
		name   string
		ranges [][2]int
	}{
		{
			name:   "all",
			ranges: [][2]int{{0, len(data)}},
		},
		{
			name:   "all without end",
			ranges: [][2]int{{0, -1}},
		},
		{
			name:   "all without start",
			ranges: [][2]int{{-1, len(data)}},
		},
		{
			name:   "head",
			ranges: [][2]int{{0, 50}},
		},
		{
			name:   "tail",
			ranges: [][2]int{{250, len(data)}},
		},
		{
			name:   "middle",
			ranges: [][2]int{{10, 15}},
		},
		{
			name:   "multiple",
			ranges: [][2]int{{10, 15}, {100, 125}},
		},
		{
			name:   "even more multiple parts",
			ranges: [][2]int{{10, 15}, {100, 125}, {250, 252}, {261, 270}, {270, 280}},
		},
	}

	for _, upload := range uploads {
		upload := upload
		t.Run(upload.name, func(t *testing.T) {
			t.Parallel()

			logger := log.Noop
			client, _, _, _ := newTestServer(t, testServerOptions{
				Storer: mockstorer.New(),
				Logger: logger,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})

			var resp api.BzzUploadResponse

			testOpts := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestBody(upload.reader),
				jsonhttptest.WithRequestHeader(api.ContentTypeHeader, upload.contentType),
				jsonhttptest.WithUnmarshalJSONResponse(&resp),
				jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			}
			if upload.name == "dir" {
				testOpts = append(testOpts, jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"))
			}

			jsonhttptest.Request(t, client, http.MethodPost, upload.uploadEndpoint, http.StatusCreated,
				testOpts...,
			)

			var downloadPath string
			if upload.downloadEndpoint != "/bytes" {
				downloadPath = upload.downloadEndpoint + "/" + resp.Reference.String() + "/" + upload.filepath
			} else {
				downloadPath = upload.downloadEndpoint + "/" + resp.Reference.String()
			}

			for _, tc := range ranges {
				t.Run(tc.name, func(t *testing.T) {
					rangeHeader, want := createRangeHeader(data, tc.ranges)

					var body []byte
					respHeaders := jsonhttptest.Request(t, client, http.MethodGet,
						downloadPath,
						http.StatusPartialContent,
						jsonhttptest.WithRequestHeader(api.RangeHeader, rangeHeader),
						jsonhttptest.WithPutResponseBody(&body),
					)

					got := parseRangeParts(t, respHeaders.Get(api.ContentTypeHeader), body)

					if len(got) != len(want) {
						t.Fatalf("got %v parts, want %v parts", len(got), len(want))
					}
					for i := 0; i < len(want); i++ {
						if !bytes.Equal(got[i], want[i]) {
							t.Errorf("part %v: got %q, want %q", i, string(got[i]), string(want[i]))
						}
					}
				})
			}
		})
	}
}

func createRangeHeader(data []byte, ranges [][2]int) (header string, parts [][]byte) {
	header = "bytes="
	for i, r := range ranges {
		if i > 0 {
			header += ", "
		}
		if r[0] >= 0 && r[1] >= 0 {
			parts = append(parts, data[r[0]:r[1]])
			// Range: <unit>=<range-start>-<range-end>, end is inclusive
			header += fmt.Sprintf("%v-%v", r[0], r[1]-1)
		} else {
			if r[0] >= 0 {
				header += strconv.Itoa(r[0]) // Range: <unit>=<range-start>-
				parts = append(parts, data[r[0]:])
			}
			header += "-"
			if r[1] >= 0 {
				if r[0] >= 0 {
					// Range: <unit>=<range-start>-<range-end>, end is inclusive
					header += strconv.Itoa(r[1] - 1)
				} else {
					// Range: <unit>=-<suffix-length>, the parameter is length
					header += strconv.Itoa(r[1])
				}
				parts = append(parts, data[:r[1]])
			}
		}
	}
	return
}

func parseRangeParts(t *testing.T, contentType string, body []byte) (parts [][]byte) {
	t.Helper()

	mimetype, params, _ := mime.ParseMediaType(contentType)
	if mimetype != "multipart/byteranges" {
		parts = append(parts, body)
		return
	}
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for part, err := mr.NextPart(); err == nil; part, err = mr.NextPart() {
		value, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, value)
	}
	return parts
}

func TestFeedIndirection(t *testing.T) {
	t.Parallel()

	// first, "upload" some content for the update
	var (
		updateData      = []byte("<h1>Swarm Feeds Hello World!</h1>")
		logger          = log.Noop
		storer          = mockstorer.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer: storer,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)
	// tar all the test case files
	tarReader := tarFiles(t, []f{
		{
			data:     updateData,
			name:     "index.html",
			dir:      "",
			filePath: "./index.html",
		},
	})

	var resp api.BzzUploadResponse

	options := []jsonhttptest.Option{
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(tarReader),
		jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"),
		jsonhttptest.WithUnmarshalJSONResponse(&resp),
		jsonhttptest.WithRequestHeader(api.SwarmIndexDocumentHeader, "index.html"),
	}

	// verify directory tar upload response
	jsonhttptest.Request(t, client, http.MethodPost, "/bzz", http.StatusCreated, options...)

	if resp.Reference.String() == "" {
		t.Fatalf("expected file reference, did not got any")
	}

	// now use the "content" to mock the feed lookup
	// also, use the mocked mantaray chunks that unmarshal
	// into a real manifest with the mocked feed values when
	// called from the bzz endpoint. then call the bzz endpoint with
	// the pregenerated feed root manifest hash

	feedUpdate := toChunk(t, 121212, resp.Reference.Bytes())

	var (
		look                = newMockLookup(-1, 0, feedUpdate, nil, &id{}, nil)
		factory             = newMockFactory(look)
		bzzDownloadResource = func(addr, path string) string { return "/bzz/" + addr + "/" + path }
		ctx                 = context.Background()
	)
	client, _, _, _ = newTestServer(t, testServerOptions{
		Storer: storer,
		Logger: logger,
		Feeds:  factory,
	})
	err := storer.Cache().Put(ctx, feedUpdate)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.NewDefaultManifest(
		loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false, 0)),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	emptyAddr := make([]byte, 32)
	err = m.Add(ctx, manifest.RootPath, manifest.NewEntry(swarm.NewAddress(emptyAddr), map[string]string{
		api.FeedMetadataEntryOwner: "8d3766440f0d7b949a5e32995d09619a7f86e632",
		api.FeedMetadataEntryTopic: "abcc",
		api.FeedMetadataEntryType:  "epoch",
	}))
	if err != nil {
		t.Fatal(err)
	}
	manifRef, err := m.Store(ctx)
	if err != nil {
		t.Fatal(err)
	}

	jsonhttptest.Request(t, client, http.MethodGet, bzzDownloadResource(manifRef.String(), ""), http.StatusOK,
		jsonhttptest.WithExpectedResponse(updateData),
		jsonhttptest.WithExpectedContentLength(len(updateData)),
	)
}

func Test_bzzDownloadHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		address string
		want    jsonhttp.StatusResponse
	}{{
		name:    "address - odd hex string",
		address: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "address - invalid hex character",
		address: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "address",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, fmt.Sprintf("/bzz/%s/abc", tc.address), tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func TestInvalidBzzParams(t *testing.T) {
	t.Parallel()

	var (
		fileUploadResource = "/bzz"
		storerMock         = mockstorer.New()
		logger             = log.Noop
		existsFn           = func(id []byte) (bool, error) {
			return false, errors.New("error")
		}
	)

	t.Run("batch unusable", func(t *testing.T) {
		t.Parallel()

		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
		})
		clientBatchUnusable, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(),
		})
		jsonhttptest.Request(t, clientBatchUnusable, http.MethodPost, fileUploadResource, http.StatusUnprocessableEntity,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		)

	})

	t.Run("batch exists", func(t *testing.T) {
		t.Parallel()

		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
		})
		clientBatchExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(mockbatchstore.WithExistsFunc(existsFn)),
		})
		jsonhttptest.Request(t, clientBatchExists, http.MethodPost, fileUploadResource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		)

	})

	t.Run("batch not found", func(t *testing.T) {
		t.Parallel()

		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
		})
		clientBatchExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(),
		})
		jsonhttptest.Request(t, clientBatchExists, http.MethodPost, fileUploadResource, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		)
	})

	t.Run("upload, invalid tag", func(t *testing.T) {
		t.Parallel()

		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
		})
		clientInvalidTag, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		jsonhttptest.Request(t, clientInvalidTag, http.MethodPost, fileUploadResource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, "tag"),
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar))
	})

	t.Run("upload, tag not found", func(t *testing.T) {
		t.Parallel()

		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					api.ContentTypeHeader: {"text/plain; charset=utf-8"},
				},
			},
		})
		clientTagExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		jsonhttptest.Request(t, clientTagExists, http.MethodPost, fileUploadResource, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, strconv.FormatUint(10000, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar))
	})

	t.Run("address not found", func(t *testing.T) {
		t.Parallel()

		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		address := "f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b"
		jsonhttptest.Request(t, client, http.MethodGet, fmt.Sprintf("/bzz/%s/", address), http.StatusNotFound)
	})

}

// TestDirectUploadBzz tests that the direct upload endpoint give correct error message in dev mode
func TestDirectUploadBzz(t *testing.T) {
	t.Parallel()

	var (
		fileUploadResource = "/bzz"
		storerMock         = mockstorer.New()
		logger             = log.Noop
	)

	tr := tarFiles(t, []f{
		{
			data: []byte("robots text"),
			name: "robots.txt",
			dir:  "",
			header: http.Header{
				api.ContentTypeHeader: {"text/plain; charset=utf-8"},
			},
		},
	})
	clientBatchUnusable, _, _, _ := newTestServer(t, testServerOptions{
		Storer:     storerMock,
		Logger:     logger,
		Post:       mockpost.New(mockpost.WithAcceptAll()),
		BatchStore: mockbatchstore.New(),
		BeeMode:    api.DevMode,
	})
	jsonhttptest.Request(t, clientBatchUnusable, http.MethodPost, fileUploadResource, http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "false"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(tr),
		jsonhttptest.WithRequestHeader(api.ContentTypeHeader, api.ContentTypeTar),
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrUnsupportedDevNodeOperation.Error(),
			Code:    http.StatusBadRequest,
		}),
	)
}
