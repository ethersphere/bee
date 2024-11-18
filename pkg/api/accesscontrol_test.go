// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	mockac "github.com/ethersphere/bee/v2/pkg/accesscontrol/mock"
	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"gitlab.com/nolash/go-mockbytes"
)

//nolint:ireturn
func prepareHistoryFixture(storer api.Storer) (accesscontrol.History, swarm.Address) {
	ctx := context.Background()
	ls := loadsave.New(storer.ChunkStore(), storer.Cache(), pipelineFactory(storer.Cache(), false, redundancy.NONE))

	h, _ := accesscontrol.NewHistory(ls)

	testActRef1 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd891"))
	firstTime := time.Date(1994, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = h.Add(ctx, testActRef1, &firstTime, nil)

	testActRef2 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd892"))
	secondTime := time.Date(2000, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = h.Add(ctx, testActRef2, &secondTime, nil)

	testActRef3 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd893"))
	thirdTime := time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = h.Add(ctx, testActRef3, &thirdTime, nil)

	testActRef4 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd894"))
	fourthTime := time.Date(2020, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = h.Add(ctx, testActRef4, &fourthTime, nil)

	testActRef5 := swarm.NewAddress([]byte("39a5ea87b141fe44aa609c3327ecd895"))
	fifthTime := time.Date(2030, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
	_ = h.Add(ctx, testActRef5, &fifthTime, nil)

	ref, _ := h.Store(ctx)
	return h, ref
}

// nolint:paralleltest,tparallel
// TestAccessLogicEachEndpointWithAct [positive tests]:
// On each endpoint: upload w/ "Swarm-Act" header then download and check the decrypted data
func TestAccessLogicEachEndpointWithAct(t *testing.T) {
	t.Parallel()
	var (
		spk, _         = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _          = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher      = hex.EncodeToString(publicKeyBytes)
		testfile       = "testfile1"
		storerMock     = mockstorer.New()
		logger         = log.Noop
		now            = time.Now().Unix()
		chunk          = swarm.NewChunk(
			swarm.MustParseHexAddress("0025737be11979e91654dffd2be817ac1e52a2dadb08c97a7cef12f937e707bc"),
			[]byte{72, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 149, 179, 31, 244, 146, 247, 129, 123, 132, 248, 215, 77, 44, 47, 91, 248, 229, 215, 89, 156, 210, 243, 3, 110, 204, 74, 101, 119, 53, 53, 145, 188, 193, 153, 130, 197, 83, 152, 36, 140, 150, 209, 191, 214, 193, 4, 144, 121, 32, 45, 205, 220, 59, 227, 28, 43, 161, 51, 108, 14, 106, 180, 135, 2},
		)
		g           = mockbytes.New(0, mockbytes.MockTypeStandard).WithModulus(255)
		bytedata, _ = g.SequentialBytes(swarm.ChunkSize * 2)
		tag, _      = storerMock.NewSession()
		sch         = testingsoc.GenerateMockSOCWithKey(t, []byte("foo"), pk)
		dirdata     = []byte("Lorem ipsum dolor sit amet")
		socResource = func(owner, id, sig string) string { return fmt.Sprintf("/soc/%s/%s?sig=%s", owner, id, sig) }
	)

	tc := []struct {
		name        string
		downurl     string
		upurl       string
		exphash     string
		data        io.Reader
		expdata     []byte
		contenttype string
		resp        struct {
			Reference swarm.Address `json:"reference"`
		}
		direct bool
	}{
		{
			name:        "bzz",
			upurl:       "/bzz?name=sample.html",
			downurl:     "/bzz",
			exphash:     "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade",
			resp:        api.BzzUploadResponse{Reference: swarm.MustParseHexAddress("a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade")},
			data:        strings.NewReader(testfile),
			expdata:     []byte(testfile),
			contenttype: "text/html; charset=utf-8",
		},
		{
			name:    "bzz-dir",
			upurl:   "/bzz?name=ipsum/lorem.txt",
			downurl: "/bzz",
			exphash: "6561b2a744d2a8f276270585da22e092c07c56624af83ac9969d52b54e87cee6/ipsum/lorem.txt",
			resp:    api.BzzUploadResponse{Reference: swarm.MustParseHexAddress("6561b2a744d2a8f276270585da22e092c07c56624af83ac9969d52b54e87cee6")},
			data: tarFiles(t, []f{
				{
					data: dirdata,
					name: "lorem.txt",
					dir:  "ipsum",
					header: http.Header{
						api.ContentTypeHeader: {"text/plain; charset=utf-8"},
					},
				},
			}),
			expdata:     dirdata,
			contenttype: api.ContentTypeTar,
		},
		{
			name:        "bytes",
			upurl:       "/bytes",
			downurl:     "/bytes",
			exphash:     "e30da540bb9e1901169977fcf617f28b7f8df4537de978784f6d47491619a630",
			resp:        api.BytesPostResponse{Reference: swarm.MustParseHexAddress("e30da540bb9e1901169977fcf617f28b7f8df4537de978784f6d47491619a630")},
			data:        bytes.NewReader(bytedata),
			expdata:     bytedata,
			contenttype: "application/octet-stream",
		},
		{
			name:        "chunks",
			upurl:       "/chunks",
			downurl:     "/chunks",
			exphash:     "ca8d2d29466e017cba46d383e7e0794d99a141185ec525086037f25fc2093155",
			resp:        api.ChunkAddressResponse{Reference: swarm.MustParseHexAddress("ca8d2d29466e017cba46d383e7e0794d99a141185ec525086037f25fc2093155")},
			data:        bytes.NewReader(chunk.Data()),
			expdata:     chunk.Data(),
			contenttype: "binary/octet-stream",
		},
		{
			name:        "soc",
			upurl:       socResource(hex.EncodeToString(sch.Owner), hex.EncodeToString(sch.ID), hex.EncodeToString(sch.Signature)),
			downurl:     "/chunks",
			exphash:     "b100d7ce487426b17b98ff779fad4f2dd471d04ab1c8949dd2a1a78fe4a1524e",
			resp:        api.ChunkAddressResponse{Reference: swarm.MustParseHexAddress("b100d7ce487426b17b98ff779fad4f2dd471d04ab1c8949dd2a1a78fe4a1524e")},
			data:        bytes.NewReader(sch.WrappedChunk.Data()),
			expdata:     sch.Chunk().Data(),
			contenttype: "binary/octet-stream",
			direct:      true,
		},
	}

	for _, v := range tc {
		upTestOpts := []jsonhttptest.Option{
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmTagHeader, fmt.Sprintf("%d", tag.TagID)),
			jsonhttptest.WithRequestBody(v.data),
			jsonhttptest.WithExpectedJSONResponse(v.resp),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, v.contenttype),
		}
		if v.name == "soc" {
			upTestOpts = append(upTestOpts, jsonhttptest.WithRequestHeader(api.SwarmPinHeader, "true"))
		} else {
			upTestOpts = append(upTestOpts, jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader))
		}
		expcontenttype := v.contenttype
		if v.name == "bzz-dir" {
			expcontenttype = "text/plain; charset=utf-8"
			upTestOpts = append(upTestOpts, jsonhttptest.WithRequestHeader(api.SwarmCollectionHeader, "True"))
		}
		t.Run(v.name, func(t *testing.T) {
			client, _, _, chanStore := newTestServer(t, testServerOptions{
				Storer:        storerMock,
				Logger:        logger,
				Post:          mockpost.New(mockpost.WithAcceptAll()),
				PublicKey:     pk.PublicKey,
				AccessControl: mockac.New(),
				DirectUpload:  v.direct,
			})

			if chanStore != nil {
				chanStore.Subscribe(func(chunk swarm.Chunk) {
					err := storerMock.Put(context.Background(), chunk)
					if err != nil {
						t.Fatal(err)
					}
				})
			}

			header := jsonhttptest.Request(t, client, http.MethodPost, v.upurl, http.StatusCreated,
				upTestOpts...,
			)

			historyRef := header.Get(api.SwarmActHistoryAddressHeader)
			jsonhttptest.Request(t, client, http.MethodGet, v.downurl+"/"+v.exphash, http.StatusOK,
				jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
				jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, historyRef),
				jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
				jsonhttptest.WithExpectedResponse(v.expdata),
				jsonhttptest.WithExpectedContentLength(len(v.expdata)),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, expcontenttype),
			)

			if v.name != "bzz-dir" && v.name != "soc" && v.name != "chunks" {
				t.Run("head", func(t *testing.T) {
					jsonhttptest.Request(t, client, http.MethodHead, v.downurl+"/"+v.exphash, http.StatusOK,
						jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
						jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, historyRef),
						jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
						jsonhttptest.WithRequestBody(v.data),
						jsonhttptest.WithExpectedContentLength(len(v.expdata)),
						jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, expcontenttype),
					)
				})
			}
		})
	}
}

// TestAccessLogicWithoutActHeader [negative tests]:
// 1. upload w/ "Swarm-Act" header then try to download w/o the header.
// 2. upload w/o "Swarm-Act" header then try to download w/ the header.
//
//nolint:paralleltest,tparallel
func TestAccessLogicWithoutAct(t *testing.T) {
	t.Parallel()
	var (
		spk, _               = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _                = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes       = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher            = hex.EncodeToString(publicKeyBytes)
		fileUploadResource   = "/bzz"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		storerMock           = mockstorer.New()
		h, fixtureHref       = prepareHistoryFixture(storerMock)
		logger               = log.Noop
		fileName             = "sample.html"
		now                  = time.Now().Unix()
	)

	t.Run("upload-w/-act-then-download-w/o-act", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		var (
			testfile     = "testfile1"
			encryptedRef = "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		)
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "address not found or incorrect",
				Code:    http.StatusNotFound,
			}),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})

	t.Run("upload-w/o-act-then-download-w/-act", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(),
		})
		var (
			rootHash   = "0cb947ccbc410c43139ba4409d83bf89114cb0d79556a651c06c888cf73f4d7e"
			sampleHtml = `<!DOCTYPE html>
			<html>
			<body>

			<h1>My First Heading</h1>

			<p>My first paragraph.</p>

			</body>
			</html>`
		)

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

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(rootHash), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "act or history entry not found",
				Code:    http.StatusNotFound,
			}),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})
}

// TestAccessLogicInvalidPath [negative test]: Expect Bad request when the path address is invalid.
//
//nolint:paralleltest,tparallel
func TestAccessLogicInvalidPath(t *testing.T) {
	t.Parallel()
	var (
		spk, _               = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _                = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes       = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher            = hex.EncodeToString(publicKeyBytes)
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		storerMock           = mockstorer.New()
		_, fixtureHref       = prepareHistoryFixture(storerMock)
		logger               = log.Noop
		now                  = time.Now().Unix()
	)

	t.Run("invalid-path-params", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(),
		})
		encryptedRef := "asd"

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid path params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "address",
						Error: api.HexInvalidByteError('s').Error(),
					},
				},
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		)
	})
}

// nolint:paralleltest,tparallel
// TestAccessLogicHistory tests:
// [positive tests] 1., 2.: uploading a file w/ and w/o history address then downloading it and checking the data.
// [negative test] 3. uploading a file then downloading it with a wrong history address.
// [negative test] 4. uploading a file to a wrong history address.
// [negative test] 5. downloading a file to w/o history address.
func TestAccessLogicHistory(t *testing.T) {
	t.Parallel()
	var (
		spk, _               = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _                = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes       = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher            = hex.EncodeToString(publicKeyBytes)
		fileUploadResource   = "/bzz"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		storerMock           = mockstorer.New()
		h, fixtureHref       = prepareHistoryFixture(storerMock)
		logger               = log.Noop
		fileName             = "sample.html"
		now                  = time.Now().Unix()
	)

	t.Run("empty-history-upload-then-download-and-check-data", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(),
		})
		var (
			testfile     = "testfile1"
			encryptedRef = "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		)
		header := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		historyRef := header.Get(api.SwarmActHistoryAddressHeader)
		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, historyRef),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponse([]byte(testfile)),
			jsonhttptest.WithExpectedContentLength(len(testfile)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("with-history-upload-then-download-and-check-data", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		var (
			encryptedRef = "c611199e1b3674d6bf89a83e518bd16896bf5315109b4a23dcb4682a02d17b97"
			testfile     = `<!DOCTYPE html>
			<html>
			<body>

			<h1>My First Heading</h1>

			<p>My first paragraph.</p>

			</body>
			</html>`
		)

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponse([]byte(testfile)),
			jsonhttptest.WithExpectedContentLength(len(testfile)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("upload-then-download-wrong-history", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		var (
			testfile     = "testfile1"
			encryptedRef = "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		)
		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, "fc4e9fe978991257b897d987bc4ff13058b66ef45a53189a0b4fe84bb3346396"),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "act or history entry not found",
				Code:    http.StatusNotFound,
			}),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})

	t.Run("upload-wrong-history", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(),
		})
		testfile := "testfile1"

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "act or history entry not found",
				Code:    http.StatusNotFound,
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		)
	})

	t.Run("download-w/o-history", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		encryptedRef := "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})
}

// nolint:paralleltest,tparallel
// TestAccessLogicTimestamp
// [positive test] 1.: uploading a file w/ ACT then download it w/ timestamp and check the data.
// [negative test] 2.: try to download a file w/o timestamp.
func TestAccessLogicTimestamp(t *testing.T) {
	t.Parallel()
	var (
		spk, _               = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _                = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes       = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher            = hex.EncodeToString(publicKeyBytes)
		fileUploadResource   = "/bzz"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		storerMock           = mockstorer.New()
		h, fixtureHref       = prepareHistoryFixture(storerMock)
		logger               = log.Noop
		fileName             = "sample.html"
	)
	t.Run("upload-then-download-with-timestamp-and-check-data", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		var (
			thirdTime    = time.Date(2015, time.April, 1, 0, 0, 0, 0, time.UTC).Unix()
			encryptedRef = "c611199e1b3674d6bf89a83e518bd16896bf5315109b4a23dcb4682a02d17b97"
			testfile     = `<!DOCTYPE html>
			<html>
			<body>

			<h1>My First Heading</h1>

			<p>My first paragraph.</p>

			</body>
			</html>`
		)

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(thirdTime, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponse([]byte(testfile)),
			jsonhttptest.WithExpectedContentLength(len(testfile)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("download-w/o-timestamp", func(t *testing.T) {
		encryptedRef := "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})
	t.Run("download-w/-invalid-timestamp", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
		var (
			invalidTime  = int64(-1)
			encryptedRef = "c611199e1b3674d6bf89a83e518bd16896bf5315109b4a23dcb4682a02d17b97"
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(invalidTime, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: accesscontrol.ErrInvalidTimestamp.Error(),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		)
	})
}

// nolint:paralleltest,tparallel
// TestAccessLogicPublisher
// [positive test] 1.: uploading a file w/ ACT then download it w/ the publisher address and check the data.
// [negative test] 2.: expect Bad request when the public key is invalid.
// [negative test] 3.: try to download a file w/ an incorrect publisher address.
// [negative test] 3.: try to download a file w/o a publisher address.
func TestAccessLogicPublisher(t *testing.T) {
	t.Parallel()
	var (
		spk, _               = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _                = crypto.DecodeSecp256k1PrivateKey(spk)
		publicKeyBytes       = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
		publisher            = hex.EncodeToString(publicKeyBytes)
		fileUploadResource   = "/bzz"
		fileDownloadResource = func(addr string) string { return "/bzz/" + addr }
		storerMock           = mockstorer.New()
		h, fixtureHref       = prepareHistoryFixture(storerMock)
		logger               = log.Noop
		fileName             = "sample.html"
		now                  = time.Now().Unix()
	)

	t.Run("upload-then-download-w/-publisher-and-check-data", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String()), mockac.WithPublisher(publisher)),
		})
		var (
			encryptedRef = "a5a26b4915d7ce1622f9ca52252092cf2445f98d359dabaf52588c05911aaf4f"
			testfile     = `<!DOCTYPE html>
			<html>
			<body>

			<h1>My First Heading</h1>

			<p>My first paragraph.</p>

			</body>
			</html>`
		)

		jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponse([]byte(testfile)),
			jsonhttptest.WithExpectedContentLength(len(testfile)),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithExpectedResponseHeader(api.ContentDispositionHeader, fmt.Sprintf(`inline; filename="%s"`, fileName)),
		)
	})

	t.Run("upload-then-download-invalid-publickey", func(t *testing.T) {
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithPublisher(publisher)),
		})
		var (
			publickey    = "b786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfb"
			encryptedRef = "a5a26b4915d7ce1622f9ca52252092cf2445f98d359dabaf52588c05911aaf4f"
			testfile     = `<!DOCTYPE html>
			<html>
			<body>

			<h1>My First Heading</h1>

			<p>My first paragraph.</p>

			</body>
			</html>`
		)

		header := jsonhttptest.Request(t, client, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(api.BzzUploadResponse{
				Reference: swarm.MustParseHexAddress(encryptedRef),
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
			jsonhttptest.WithNonEmptyResponseHeader(api.SwarmTagHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ETagHeader, fmt.Sprintf("%q", encryptedRef)),
		)

		historyRef := header.Get(api.SwarmActHistoryAddressHeader)
		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, historyRef),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publickey),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid header params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "Swarm-Act-Publisher",
						Error: "malformed public key: invalid length: 32",
					},
				},
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		)
	})

	t.Run("download-w/-wrong-publisher", func(t *testing.T) {
		var (
			downloader   = "03c712a7e29bc792ac8d8ae49793d28d5bda27ed70f0d90697b2fb456c0a168bd2"
			encryptedRef = "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		)
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String()), mockac.WithPublisher(publisher)),
		})

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, downloader),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: accesscontrol.ErrInvalidPublicKey.Error(),
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})

	t.Run("re-upload-with-invalid-publickey", func(t *testing.T) {
		var (
			downloader = "03c712a7e29bc792ac8d8ae49793d28d5bda27ed70f0d90697b2fb456c0a168bd2"
			testfile   = "testfile1"
		)
		downloaderClient, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithPublisher(downloader)),
		})

		jsonhttptest.Request(t, downloaderClient, http.MethodPost, fileUploadResource+"?name="+fileName, http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmActHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(strings.NewReader(testfile)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid public key",
			}),
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "text/html; charset=utf-8"),
		)

	})

	t.Run("download-w/o-publisher", func(t *testing.T) {
		encryptedRef := "a5df670544eaea29e61b19d8739faa4573b19e4426e58a173e51ed0b5e7e2ade"
		client, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String()), mockac.WithPublisher(publisher)),
		})

		jsonhttptest.Request(t, client, http.MethodGet, fileDownloadResource(encryptedRef), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmActTimestampHeader, strconv.FormatInt(now, 10)),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, fixtureHref.String()),
			jsonhttptest.WithRequestHeader(api.SwarmActPublisherHeader, publisher),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
		)
	})
}

func TestAccessLogicGrantees(t *testing.T) {
	t.Parallel()
	var (
		spk, _          = hex.DecodeString("a786dd84b61485de12146fd9c4c02d87e8fd95f0542765cb7fc3d2e428c0bcfa")
		pk, _           = crypto.DecodeSecp256k1PrivateKey(spk)
		storerMock      = mockstorer.New()
		h, fixtureHref  = prepareHistoryFixture(storerMock)
		logger          = log.Noop
		addr            = swarm.RandAddress(t)
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String())),
		})
	)
	t.Run("get-grantees", func(t *testing.T) {
		var (
			publicKeyBytes = crypto.EncodeSecp256k1PublicKey(&pk.PublicKey)
			publisher      = hex.EncodeToString(publicKeyBytes)
		)
		clientwihtpublisher, _, _, _ := newTestServer(t, testServerOptions{
			Storer:        storerMock,
			Logger:        logger,
			Post:          mockpost.New(mockpost.WithAcceptAll()),
			PublicKey:     pk.PublicKey,
			AccessControl: mockac.New(mockac.WithHistory(h, fixtureHref.String()), mockac.WithPublisher(publisher)),
		})
		expected := []string{
			"03d7660772cc3142f8a7a2dfac46ce34d12eac1718720cef0e3d94347902aa96a2",
			"03c712a7e29bc792ac8d8ae49793d28d5bda27ed70f0d90697b2fb456c0a168bd2",
			"032541acf966823bae26c2c16a7102e728ade3e2e29c11a8a17b29d8eb2bd19302",
		}
		jsonhttptest.Request(t, clientwihtpublisher, http.MethodGet, "/grantee/"+addr.String(), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(expected),
		)
	})

	t.Run("get-grantees-unauthorized", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/grantee/fc4e9fe978991257b897d987bc4ff13058b66ef45a53189a0b4fe84bb3346396", http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "granteelist not found",
				Code:    http.StatusNotFound,
			}),
		)
	})
	t.Run("get-grantees-invalid-address", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/grantee/asd", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid path params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "address",
						Error: api.HexInvalidByteError('s').Error(),
					},
				},
			}),
		)
	})
	t.Run("add-revoke-grantees", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Addlist:    []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
			Revokelist: []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, addr.String()),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("add-revoke-grantees-wrong-history", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Addlist:    []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
			Revokelist: []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusNotFound,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, swarm.EmptyAddress.String()),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "act or history entry not found",
				Code:    http.StatusNotFound,
			}),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("invlaid-add-grantees", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Addlist: []string{"random-string"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, addr.String()),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid add list",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("invlaid-revoke-grantees", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Revokelist: []string{"random-string"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, addr.String()),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid revoke list",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("add-revoke-grantees-empty-body", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(nil)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "could not validate request",
				Code:    http.StatusBadRequest,
			}),
		)
	})
	t.Run("add-grantee-with-history", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Addlist: []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusOK,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestHeader(api.SwarmActHistoryAddressHeader, addr.String()),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("add-grantee-without-history", func(t *testing.T) {
		body := api.GranteesPatchRequest{
			Addlist: []string{"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4"},
		}
		jsonhttptest.Request(t, client, http.MethodPatch, "/grantee/"+addr.String(), http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})

	t.Run("create-granteelist", func(t *testing.T) {
		body := api.GranteesPostRequest{
			GranteeList: []string{
				"02ab7473879005929d10ce7d4f626412dad9fe56b0a6622038931d26bd79abf0a4",
				"03d7660772cc3142f8a7a2dfac46ce34d12eac1718720cef0e3d94347902aa96a2",
			},
		}
		jsonhttptest.Request(t, client, http.MethodPost, "/grantee", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("create-granteelist-without-stamp", func(t *testing.T) {
		body := api.GranteesPostRequest{
			GranteeList: []string{
				"03d7660772cc3142f8a7a2dfac46ce34d12eac1718720cef0e3d94347902aa96a2",
			},
		}
		jsonhttptest.Request(t, client, http.MethodPost, "/grantee", http.StatusBadRequest,
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
	t.Run("create-granteelist-empty-body", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, "/grantee", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(nil)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "could not validate request",
				Code:    http.StatusBadRequest,
			}),
		)
	})
	t.Run("create-granteelist-invalid-body", func(t *testing.T) {
		body := api.GranteesPostRequest{
			GranteeList: []string{"random-string"},
		}
		jsonhttptest.Request(t, client, http.MethodPost, "/grantee", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithRequestBody(bytes.NewReader(nil)),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid grantee list",
				Code:    http.StatusBadRequest,
			}),
			jsonhttptest.WithJSONRequestBody(body),
		)
	})
}
