// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	smock "github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestBzzFiles(t *testing.T) {
	var (
		fileUploadResource   = "/bzz"
		targets              = "0x222"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		simpleData           = []byte("this is a simple text")
		storerMock           = smock.NewStorer()
		statestoreMock       = statestore.NewStateStore()
		pinningMock          = pinning.NewServiceMock()
		logger               = logging.New(ioutil.Discard, 0)
		client, _, _         = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Pinning: pinningMock,
			Tags:    tags.NewTags(statestoreMock, logger),
			Logger:  logger,
			Post:    mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	t.Run("invalid-content-type", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource,
			http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.InvalidContentType.Error(),
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("tar-file-upload", func(t *testing.T) {
		tr := tarFiles(t, []f{
			{
				data: []byte("robots text"),
				name: "robots.txt",
				dir:  "",
				header: http.Header{
					"Content-Type": {"text/plain; charset=utf-8"},
				},
			},
			{
				data: []byte("image 1"),
				name: "1.png",
				dir:  "img",
				header: http.Header{
					"Content-Type": {"image/png"},
				},
			},
			{
				data: []byte("image 2"),
				name: "2.png",
				dir:  "img",
				header: http.Header{
					"Content-Type": {"image/png"},
				},
			},
		})
		address := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: address,
			}),
		)

		has, err := storerMock.Has(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk address: have none; want one")
		}

		refs, err := pinningMock.Pins()
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
					"Content-Type": {"text/plain; charset=utf-8"},
				},
			},
			{
				data: []byte("image 1"),
				name: "1.png",
				dir:  "img",
				header: http.Header{
					"Content-Type": {"image/png"},
				},
			},
			{
				data: []byte("image 2"),
				name: "2.png",
				dir:  "img",
				header: http.Header{
					"Content-Type": {"image/png"},
				},
			},
		})
		reference := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithRequestBody(tr),
			jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: reference,
			}),
		)

		has, err := storerMock.Has(context.Background(), reference)
		if err != nil {
			t.Fatal(err)
		}
		if !has {
			t.Fatal("storer check root chunk reference: have none; want one")
		}

		refs, err := pinningMock.Pins()
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
		jsonhttptest.Request(t, client, http.MethodPost,
			fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, "True"),
			jsonhttptest.WithRequestHeader("Content-Type", "image/jpeg; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		rootHash := resp.Reference.String()
		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet,
			fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedResponse(simpleData),
		)
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid file name detected")
		}
		if rcvdHeader.Get("Content-Type") != "image/jpeg; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}
	})

	t.Run("check-content-type-detection", func(t *testing.T) {
		fileName := "my-pictures.jpeg"
		rootHash := "4f9146b3813ccbd7ce45a18be23763d7e436ab7a3982ef39961c6f3cd4da1dcf"

		jsonhttptest.Request(t, client, http.MethodPost,
			fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader("Content-Type", "image/jpeg; charset=utf-8"),
		)

		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet,
			fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedResponse(simpleData),
		)
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid file name detected")
		}
		if rcvdHeader.Get("Content-Type") != "image/jpeg; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}
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

		rcvdHeader := jsonhttptest.Request(t, client, http.MethodPost,
			fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(sampleHtml)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader("Content-Type", "text/html; charset=utf-8"),
		)

		if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", rootHash) {
			t.Fatal("Invalid ETags header received")
		}

		// try to fetch the same file and check the data
		rcvdHeader = jsonhttptest.Request(t, client, http.MethodGet,
			fileDownloadResource(rootHash), http.StatusOK,
			jsonhttptest.WithExpectedResponse([]byte(sampleHtml)),
		)

		// check the headers
		cd := rcvdHeader.Get("Content-Disposition")
		_, params, err := mime.ParseMediaType(cd)
		if err != nil {
			t.Fatal(err)
		}
		if params["filename"] != fileName {
			t.Fatal("Invalid filename detected")
		}
		if rcvdHeader.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatal("Invalid content type detected")
		}

	})

	t.Run("upload-then-download-with-targets", func(t *testing.T) {
		fileName := "simple_file.txt"
		rootHash := "65148cd89b58e91616773f5acea433f7b5a6274f2259e25f4893a332b74a7e28"

		jsonhttptest.Request(t, client, http.MethodPost,
			fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader("Content-Type", "text/html; charset=utf-8"),
		)

		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet,
			fileDownloadResource(rootHash)+"?targets="+targets, http.StatusOK,
			jsonhttptest.WithExpectedResponse(simpleData),
		)

		if rcvdHeader.Get(api.TargetsRecoveryHeader) != targets {
			t.Fatalf("targets mismatch. got %s, want %s",
				rcvdHeader.Get(api.TargetsRecoveryHeader), targets)
		}
	})

}

// TestRangeRequests validates that all endpoints are serving content with
// respect to HTTP Range headers.
func TestBzzFilesRangeRequests(t *testing.T) {
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
						"Content-Type": {"text/plain; charset=utf-8"},
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
		t.Run(upload.name, func(t *testing.T) {
			mockStatestore := statestore.NewStateStore()
			logger := logging.New(ioutil.Discard, 0)
			client, _, _ := newTestServer(t, testServerOptions{
				Storer: smock.NewStorer(),
				Tags:   tags.NewTags(mockStatestore, logger),
				Logger: logger,
				Post:   mockpost.New(mockpost.WithAcceptAll()),
			})

			var resp api.BzzUploadResponse

			testOpts := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestBody(upload.reader),
				jsonhttptest.WithRequestHeader("Content-Type", upload.contentType),
				jsonhttptest.WithUnmarshalJSONResponse(&resp),
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
						jsonhttptest.WithRequestHeader("Range", rangeHeader),
						jsonhttptest.WithPutResponseBody(&body),
					)

					got := parseRangeParts(t, respHeaders.Get("Content-Type"), body)

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
		value, err := ioutil.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, value)
	}
	return parts
}

func TestFeedIndirection(t *testing.T) {
	// first, "upload" some content for the update
	var (
		updateData     = []byte("<h1>Swarm Feeds Hello World!</h1>")
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		storer         = smock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: storer,
			Tags:   tags.NewTags(mockStatestore, logger),
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
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithRequestBody(tarReader),
		jsonhttptest.WithRequestHeader("Content-Type", api.ContentTypeTar),
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
	client, _, _ = newTestServer(t, testServerOptions{
		Storer: storer,
		Tags:   tags.NewTags(mockStatestore, logger),
		Logger: logger,
		Feeds:  factory,
	})
	_, err := storer.Put(ctx, storage.ModePutUpload, feedUpdate)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.NewDefaultManifest(
		loadsave.New(storer, pipelineFactory(storer, storage.ModePutUpload, false)),
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
	)
}

func TestBzzReupload(t *testing.T) {
	var (
		logger         = logging.New(ioutil.Discard, 0)
		mockStatestore = statestore.NewStateStore()
		m              = &mockSteward{}
		storer         = smock.NewStorer()
		addr           = swarm.NewAddress([]byte{31: 128})
	)
	client, _, _ := newTestServer(t, testServerOptions{
		Storer:  storer,
		Tags:    tags.NewTags(mockStatestore, logger),
		Logger:  logger,
		Steward: m,
	})
	jsonhttptest.Request(t, client, http.MethodPatch, "/v1/bzz/"+addr.String(), http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		}),
	)
	if !m.addr.Equal(addr) {
		t.Fatalf("got address %s want %s", m.addr.String(), addr.String())
	}
}
