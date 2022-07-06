// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	pinning "github.com/ethersphere/bee/pkg/pinning/mock"
	mockbatchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"gitlab.com/nolash/go-mockbytes"
)

// TestBytes tests that the data upload api responds as expected when uploading,
// downloading and requesting a resource that cannot be found.
func TestBytes(t *testing.T) {
	const (
		resource = "/bytes"
		expHash  = "29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9"
	)

	var (
		storerMock      = mock.NewStorer()
		pinningMock     = pinning.NewServiceMock()
		logger          = logging.New(io.Discard, 0)
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:  storerMock,
			Tags:    tags.NewTags(statestore.NewStateStore(), logging.New(io.Discard, 0)),
			Pinning: pinningMock,
			Logger:  logger,
			Post:    mockpost.New(mockpost.WithAcceptAll()),
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
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
			jsonhttptest.WithExpectedJSONResponse(api.BytesPostResponse{
				Reference: chunkAddr,
			}),
		)

		has, err := storerMock.Has(context.Background(), chunkAddr)
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

	t.Run("download", func(t *testing.T) {
		resp := request(t, client, http.MethodGet, resource+"/"+expHash, nil, http.StatusOK)
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, content) {
			t.Fatalf("data mismatch. got %s, want %s", string(data), string(content))
		}
	})

	t.Run("head", func(t *testing.T) {
		resp := request(t, client, http.MethodHead, resource+"/"+expHash, nil, http.StatusOK)
		if int(resp.ContentLength) != len(content) {
			t.Fatalf("length %d want %d", resp.ContentLength, len(content))
		}
	})
	t.Run("head with compression", func(t *testing.T) {
		resp := jsonhttptest.Request(t, client, http.MethodHead, resource+"/"+expHash, http.StatusOK,
			jsonhttptest.WithRequestHeader("Accept-Encoding", "gzip"),
		)
		val, err := strconv.Atoi(resp.Get("Content-Length"))
		if err != nil {
			t.Fatal(err)
		}
		if val != len(content) {
			t.Fatalf("length %d want %d", val, len(content))
		}
	})

	t.Run("not found", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, resource+"/0xabcd", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Not Found",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("internal error", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, resource+"/abcd", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Internal Server Error",
				Code:    http.StatusInternalServerError,
			}),
		)
	})

	t.Run("upload multipart error", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW"),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)
	})
}

func TestBytesInvalidStamp(t *testing.T) {
	const (
		resource = "/bytes"
		expHash  = "29a5fb121ce96194ba8b7b823a1f9c6af87e1791f824940a53b5a7efe3f790d9"
	)

	var (
		storerMock        = mock.NewStorer()
		pinningMock       = pinning.NewServiceMock()
		logger            = logging.New(io.Discard, 0)
		retBool           = false
		retErr      error = nil
		existsFn          = func(id []byte) (bool, error) {
			return retBool, retErr
		}
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:     storerMock,
			Tags:       tags.NewTags(statestore.NewStateStore(), logging.New(io.Discard, 0)),
			Pinning:    pinningMock,
			Logger:     logger,
			Post:       mockpost.New(mockpost.WithAcceptAll()),
			BatchStore: mockbatchstore.New(mockbatchstore.WithExistsFunc(existsFn)),
		})
	)

	g := mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
	content, err := g.SequentialBytes(swarm.ChunkSize * 2)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("upload, batch doesn't exist", func(t *testing.T) {
		chunkAddr := swarm.MustParseHexAddress(expHash)
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)

		has, err := storerMock.Has(context.Background(), chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatal("storer check root chunk address: have ont; want none")
		}

		refs, err := pinningMock.Pins()
		if err != nil {
			t.Fatal("unable to get pinned references")
		}
		if have, want := len(refs), 0; have != want {
			t.Fatalf("root pin count mismatch: have %d; want %d", have, want)
		}
	})

	// throw back an error
	retErr = errors.New("err happened")

	t.Run("upload, batch exists error", func(t *testing.T) {
		chunkAddr := swarm.MustParseHexAddress(expHash)
		jsonhttptest.Request(t, client, http.MethodPost, resource, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(content)),
		)

		has, err := storerMock.Has(context.Background(), chunkAddr)
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatal("storer check root chunk address: have ont; want none")
		}
	})
}
