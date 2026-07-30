// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockbatchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"gitlab.com/nolash/go-mockbytes"
)

// nolint:paralleltest,tparallel
// TestBytes tests that the data upload api responds as expected when uploading,
// downloading and requesting a resource that cannot be found.
func TestBytes(t *testing.T) {
	t.Parallel()

	const (
		resource = "/bytes"
		expHash  = "29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9"
	)

	var (
		storerMock      = mockstorer.New()
		logger          = log.Noop
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize * 2)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("upload", func(t *testing.T) {
		chunkAddr := swarm.MustParseHexAddress(expHash)
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, "0"),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			jsonhttptest.WithExpectedJSONResponse(api.BytesPostResponse{
				Reference: chunkAddr,
			}),
		)

		has, err := storerMock.ChunkStore().Has(context.Background(), chunkAddr)
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

	t.Run("upload-with-pinning", func(t *testing.T) {
		var res api.BytesPostResponse
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithUnmarshalJSONResponse(&res),
		)
		reference := res.Reference

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

	t.Run("download", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, resource+"/"+expHash, http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(content)),
			jsonhttptest.WithExpectedResponse(content),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
		)
	})

	t.Run("head", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodHead, resource+"/"+expHash, http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(content)),
		)
	})
	t.Run("head with compression", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodHead, resource+"/"+expHash, http.StatusOK,
			jsonhttptest.WithRequestHeader(api.AcceptEncodingHeader, "gzip"),
			jsonhttptest.WithExpectedContentLength(len(content)),
		)
	})

	t.Run("internal error", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, resource+"/abcd", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "joiner failed",
				Code:    http.StatusInternalServerError,
			}),
		)
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, resource+"/"+swarm.EmptyAddress.String(), http.StatusNotFound)
	})
}

// nolint:paralleltest,tparallel
func TestBytesInvalidStamp(t *testing.T) {
	t.Parallel()

	const (
		resource = "/bytes"
		expHash  = "29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9"
	)

	var (
		storerMock        = mockstorer.New()
		logger            = log.Noop
		retBool           = false
		retErr     error  = nil
		invalidTag uint64 = 100
		existsFn          = func(id []byte) (bool, error) {
			return retBool, retErr
		}
	)

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize * 2)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("upload batch not found", func(t *testing.T) {
		clientBatchNotExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(),
			BatchStore: mockbatchstore.New(mockbatchstore.WithExistsFunc(existsFn)),
		})
		chunkAddr := swarm.MustParseHexAddress(expHash)

		jsonhttptest.Request(t, clientBatchNotExists, http.MethodPost, resource, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)

		has, err := storerMock.ChunkStore().Has(context.Background(), chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatal("storer check root chunk address: have ont; want none")
		}

		refs, err := storerMock.Pins()
		if err != nil {
			t.Fatal("unable to get pinned references")
		}
		if have, want := len(refs), 0; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
	})

	// throw back an error
	retErr = errors.New("err happened")

	t.Run("upload batch exists error", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(mockbatchstore.WithExistsFunc(existsFn)),
		})

		chunkAddr := swarm.MustParseHexAddress(expHash)
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)

		has, err := storerMock.ChunkStore().Has(context.Background(), chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatal("storer check root chunk address: have ont; want none")
		}
	})

	t.Run("upload batch unusable", func(t *testing.T) {
		clientBatchUnusable, _, _, _ := newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(),
		})

		jsonhttptest.Request(t, clientBatchUnusable, http.MethodPost, resource, http.StatusUnprocessableEntity,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)
	})

	t.Run("upload invalid tag", func(t *testing.T) {
		clientInvalidTag, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		jsonhttptest.Request(t, clientInvalidTag, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, "tag"),
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)
	})

	t.Run("upload tag not found", func(t *testing.T) {
		clientTagExists, _, _, _ := newTestServer(t, testServerOptions{
			Storer: storerMock,
			Logger: logger,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})

		jsonhttptest.Request(t, clientTagExists, http.MethodPost, resource, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, strconv.FormatUint(invalidTag, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)
	})
}

func TestBytesUploadHandlerInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
	})

	tests := []struct {
		name   string
		hdrKey string
		hdrVal string
		want   jsonhttp.StatusResponse
	}{
		{
			name:   "no stamp",
			hdrKey: api.SwarmTagHeader,
			hdrVal: strconv.FormatUint(1, 10),
			want: jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "swarm-postage-batch-id",
						Error: "want required:",
					},
				},
			},
		},
		{
			name:   "invalid stamp",
			hdrKey: api.SwarmPostageBatchIdHeader,
			hdrVal: batchOkStr,
			want: jsonhttp.StatusResponse{
				Code:    http.StatusNotFound,
				Message: "batch with id not found",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPost, "/bytes", tc.want.Code,
				jsonhttptest.WithRequestHeader(tc.hdrKey, tc.hdrVal),
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func TestBytesGetHandlerInvalidInputs(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/bytes/"+tc.address, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func TestBytesRedundancyLevel(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	const maxValidLevel = redundancy.PARANOID

	tests := []struct {
		name  string
		level int
		want  *jsonhttp.StatusResponse
	}{
		{"minimum level (NONE) is valid", int(redundancy.NONE), nil},
		{"maximum valid level (PARANOID) is valid", int(maxValidLevel), nil},
		{
			"level below minimum is invalid", int(-1),
			&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "Swarm-Redundancy-Level",
						Error: "invalid syntax",
					},
				},
			},
		},
		{
			"level above maximum is invalid", int(maxValidLevel + 1),
			&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "swarm-redundancy-level",
						Error: fmt.Sprintf("want redundancy level to be between %d and %d", int(redundancy.NONE), int(redundancy.PARANOID)),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []jsonhttptest.Option{
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
				jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, strconv.Itoa(tt.level)),
				jsonhttptest.WithRequestBody(bytes.NewReader([]byte("test"))),
			}
			var statusCode int
			if tt.want == nil {
				statusCode = http.StatusCreated
			} else {
				statusCode = tt.want.Code
				opts = append(opts, jsonhttptest.WithExpectedJSONResponse(*tt.want))
			}
			jsonhttptest.Request(t, client, http.MethodPost, "/bytes", statusCode, opts...)
		})
	}
}

// TestBytesHead tests that a HEAD request reports the same entity headers as a
// GET, for every redundancy level and for encrypted references. The redundancy
// level is encoded into the most significant byte of the root chunk span and has
// to be stripped before the length is read out of it, and an encrypted reference
// carries a decryption key that is not part of the root chunk address.
func TestBytesHead(t *testing.T) {
	t.Parallel()

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize * 10)
	if err != nil {
		t.Fatal(err)
	}

	for level := redundancy.NONE; level <= redundancy.PARANOID; level++ {
		for _, encrypt := range []bool{false, true} {
			t.Run(fmt.Sprintf("level %d encrypt %v", level, encrypt), func(t *testing.T) {
				t.Parallel()

				client, _, _, _ := newTestServer(t, testServerOptions{
					Storer: mockstorer.New(),
					Post:   mockpost.New(mockpost.WithAcceptAll()),
				})

				var resp struct {
					Reference swarm.Address `json:"reference"`
				}
				jsonhttptest.Request(t, client, http.MethodPost, "/bytes", http.StatusCreated,
					jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
					jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
					jsonhttptest.WithRequestHeader(api.SwarmRedundancyLevelHeader, strconv.Itoa(int(level))),
					jsonhttptest.WithRequestHeader(api.SwarmEncryptHeader, strconv.FormatBool(encrypt)),
					jsonhttptest.WithRequestBody(bytes.NewReader(content)),
					jsonhttptest.WithUnmarshalJSONResponse(&resp),
				)

				resource := "/bytes/" + resp.Reference.String()

				jsonhttptest.Request(t, client, http.MethodHead, resource, http.StatusOK,
					jsonhttptest.WithExpectedContentLength(len(content)),
					jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
					jsonhttptest.WithExpectedResponseHeader(api.AcceptRangesHeader, "bytes"),
				)
				jsonhttptest.Request(t, client, http.MethodGet, resource, http.StatusOK,
					jsonhttptest.WithExpectedContentLength(len(content)),
					jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
					jsonhttptest.WithExpectedResponseHeader(api.AcceptRangesHeader, "bytes"),
				)
			})
		}
	}
}

// TestBytesHeadErrorsMatchGet tests that HEAD reports the same status as GET for
// references that cannot be served, so a client probing with HEAD is not told
// something different from what the subsequent GET would do.
func TestBytesHeadErrorsMatchGet(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{
		Storer: mockstorer.New(),
		Post:   mockpost.New(mockpost.WithAcceptAll()),
	})

	tests := []struct {
		name string
		ref  string
		want int
	}{
		{"unresolvable address", "abcd", http.StatusInternalServerError},
		{"unknown address", "0000000000000000000000000000000000000000000000000000000000000001", http.StatusNotFound},
		{"invalid address", "zzzz", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := "/bytes/" + tt.ref
			jsonhttptest.Request(t, client, http.MethodHead, resource, tt.want)
			jsonhttptest.Request(t, client, http.MethodGet, resource, tt.want)
		})
	}
}
