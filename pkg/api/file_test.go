// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
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
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestFiles(t *testing.T) {
	var (
		fileUploadResource   = "/files"
		targets              = "0x222"
		fileDownloadResource = func(addr string) string { return "/files/" + addr }
		simpleData           = []byte("this is a simple text")
		client               = newTestServer(t, testServerOptions{
			Storer: mock.NewStorer(),
			Tags:   tags.NewTags(),
		})
	)

	t.Run("invalid-content-type", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusBadRequest,
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid content-type header",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("multipart-upload", func(t *testing.T) {
		fileName := "simple_file.txt"
		rootHash := "295673cf7aa55d119dd6f82528c91d45b53dd63dc2e4ca4abf4ed8b3a0788085"
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusOK,
			jsonhttptest.WithMultipartRequest(bytes.NewReader(simpleData), len(simpleData), fileName, ""),
			jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
		)
	})

	t.Run("encrypt-decrypt", func(t *testing.T) {
		t.Skip("reenable after crypto refactor")
		fileName := "my-pictures.jpeg"

		var resp api.FileUploadResponse
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithRequestHeader(api.EncryptHeader, "True"),
			jsonhttptest.WithRequestHeader("Content-Type", "image/jpeg; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		rootHash := resp.Reference.String()
		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
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
		rootHash := "f2e761160deda91c1fbfab065a5abf530b0766b3e102b51fbd626ba37c3bc581"

		t.Run("binary", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusOK,
				jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
				jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
					Reference: swarm.MustParseHexAddress(rootHash),
				}),
				jsonhttptest.WithRequestHeader("Content-Type", "image/jpeg; charset=utf-8"),
			)

			rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
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

		t.Run("multipart", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusOK,
				jsonhttptest.WithMultipartRequest(bytes.NewReader(simpleData), len(simpleData), fileName, "image/jpeg; charset=utf-8"),
				jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
					Reference: swarm.MustParseHexAddress(rootHash),
				}),
			)

			rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
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
	})

	t.Run("upload-then-download-and-check-data", func(t *testing.T) {
		fileName := "sample.html"
		rootHash := "9f8ba407ff4809e877c75506247e0f1faf206262d1ddd7b3c8f9775d3501be50"
		sampleHtml := `<!DOCTYPE html>
		<html>
		<body>

		<h1>My First Heading</h1>

		<p>My first paragraph.</p>

		</body>
		</html>`

		t.Run("binary", func(t *testing.T) {
			rcvdHeader := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusOK,
				jsonhttptest.WithRequestBody(strings.NewReader(sampleHtml)),
				jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
					Reference: swarm.MustParseHexAddress(rootHash),
				}),
				jsonhttptest.WithRequestHeader("Content-Type", "text/html; charset=utf-8"),
			)

			if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", rootHash) {
				t.Fatal("Invalid ETags header received")
			}

			// try to fetch the same file and check the data
			rcvdHeader = jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
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

		t.Run("multipart", func(t *testing.T) {
			rcvdHeader := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource, http.StatusOK,
				jsonhttptest.WithMultipartRequest(strings.NewReader(sampleHtml), len(sampleHtml), fileName, ""),
				jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
					Reference: swarm.MustParseHexAddress(rootHash),
				}),
			)

			if rcvdHeader.Get("ETag") != fmt.Sprintf("%q", rootHash) {
				t.Fatal("Invalid ETags header received")
			}

			// try to fetch the same file and check the data
			rcvdHeader = jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusOK,
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
	})

	t.Run("upload-then-download-with-targets", func(t *testing.T) {
		fileName := "simple_file.txt"
		rootHash := "19d2e82c076031ec4e456978f839472d2f1b1b969a765420404d8d315a0c6123"

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(simpleData)),
			jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
				Reference: swarm.MustParseHexAddress(rootHash),
			}),
			jsonhttptest.WithRequestHeader("Content-Type", "text/html; charset=utf-8"),
		)

		rcvdHeader := jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash)+"?targets="+targets, http.StatusOK,
			jsonhttptest.WithExpectedResponse(simpleData),
		)

		if rcvdHeader.Get(api.TargetsRecoveryHeader) != targets {
			t.Fatalf("targets mismatch. got %s, want %s", rcvdHeader.Get(api.TargetsRecoveryHeader), targets)
		}
	})

}

// TestRangeRequests validates that all endpoints are serving content with
// respect to HTTP Range headers.
func TestRangeRequests(t *testing.T) {
	data := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vivamus dignissim tincidunt orci id aliquam. Praesent eget turpis in lectus semper consectetur et ut nibh. Nam rhoncus, augue sit amet sollicitudin lacinia, turpis tortor molestie urna, at mattis sem sapien sit amet augue. In bibendum ex vel odio dignissim interdum. Quisque hendrerit sapien et porta condimentum. Vestibulum efficitur mauris tellus, eget vestibulum sapien vulputate ac. Proin et vulputate sapien. Duis tincidunt mauris vulputate porta venenatis. Sed dictum aliquet urna, sit amet fermentum velit pellentesque vitae. Nam sed nisi ultrices, volutpat quam et, malesuada sapien. Nunc gravida non orci at rhoncus. Sed vitae dui accumsan, venenatis lectus et, mattis tellus. Proin sed mauris eu mi congue lacinia.")

	uploads := []struct {
		name             string
		uploadEndpoint   string
		downloadEndpoint string
		reference        string
		filepath         string
		reader           io.Reader
		contentType      string
	}{
		{
			name:             "bytes",
			uploadEndpoint:   "/bytes",
			downloadEndpoint: "/bytes",
			reference:        "4985af9dc3339ad3111c71651b92df7f21587391c01d3aa34a26879b9a1beb78",
			reader:           bytes.NewReader(data),
			contentType:      "text/plain; charset=utf-8",
		},
		{
			name:             "file",
			uploadEndpoint:   "/files",
			downloadEndpoint: "/files",
			reference:        "e387331d1c9d82f2cb01c47a4ffcdf2ed0c047cbe283e484a64fd61bffc410e7",
			reader:           bytes.NewReader(data),
			contentType:      "text/plain; charset=utf-8",
		},
		{
			name:             "bzz",
			uploadEndpoint:   "/dirs",
			downloadEndpoint: "/bzz",
			filepath:         "/ipsum/lorem.txt",
			reference:        "d2b1ab6fb26c1570712ca33efb30f8cbbaa994d5b85e1cf6f782bcae430eabaf",
			reader: tarFiles(t, []f{
				{
					data:      data,
					name:      "lorem.txt",
					dir:       "ipsum",
					reference: swarm.MustParseHexAddress("4985af9dc3339ad3111c71651b92df7f21587391c01d3aa34a26879b9a1beb78"),
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
			client := newTestServer(t, testServerOptions{
				Storer: mock.NewStorer(),
				Tags:   tags.NewTags(),
				Logger: logging.New(ioutil.Discard, 5),
			})

			jsonhttptest.Request(t, client, http.MethodPost, upload.uploadEndpoint, http.StatusOK,
				jsonhttptest.WithRequestBody(upload.reader),
				jsonhttptest.WithExpectedJSONResponse(api.FileUploadResponse{
					Reference: swarm.MustParseHexAddress(upload.reference),
				}),
				jsonhttptest.WithRequestHeader("Content-Type", upload.contentType),
			)

			for _, tc := range ranges {
				t.Run(tc.name, func(t *testing.T) {
					rangeHeader, want := createRangeHeader(data, tc.ranges)

					var body []byte
					respHeaders := jsonhttptest.Request(t, client, http.MethodGet, upload.downloadEndpoint+"/"+upload.reference+upload.filepath, http.StatusPartialContent,
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
			header += fmt.Sprintf("%v-%v", r[0], r[1]-1) // Range: <unit>=<range-start>-<range-end> // end is inclusive
		} else {
			if r[0] >= 0 {
				header += strconv.Itoa(r[0]) // Range: <unit>=<range-start>-
				parts = append(parts, data[r[0]:])
			}
			header += "-"
			if r[1] >= 0 {
				if r[0] >= 0 {
					header += strconv.Itoa(r[1] - 1) // Range: <unit>=<range-start>-<range-end> // end is inclusive
				} else {
					header += strconv.Itoa(r[1]) // Range: <unit>=-<suffix-length> // the parameter is length
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
