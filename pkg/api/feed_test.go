// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/splitter"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

const ownerString = "8d3766440f0d7b949a5e32995d09619a7f86e632"

var expReference = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")

func TestFeed_Get(t *testing.T) {
	t.Parallel()

	var (
		feedResource = func(owner, topic, at string) string {
			if at != "" {
				return fmt.Sprintf("/feeds/%s/%s?at=%s", owner, topic, at)
			}
			return fmt.Sprintf("/feeds/%s/%s", owner, topic)
		}
		mockStorer = mockstorer.New()
	)
	putter, err := mockStorer.Upload(context.Background(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	mockWrappedCh := testingc.FixtureChunk("0033")
	err = putter.Put(context.Background(), mockWrappedCh)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with at", func(t *testing.T) {
		t.Parallel()

		var (
			timestamp       = int64(12121212)
			ch              = toChunk(t, uint64(timestamp), mockWrappedCh.Address().Bytes())
			look            = newMockLookup(12, 0, ch, nil, &id{}, &id{})
			factory         = newMockFactory(look)
			idBytes, _      = (&id{}).MarshalBinary()
			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", "12"), http.StatusOK,
			jsonhttptest.WithExpectedResponse(mockWrappedCh.Data()[swarm.SpanSize:]),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
		)
	})

	t.Run("latest", func(t *testing.T) {
		t.Parallel()

		var (
			timestamp  = int64(12121212)
			ch         = toChunk(t, uint64(timestamp), mockWrappedCh.Address().Bytes())
			look       = newMockLookup(-1, 2, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", ""), http.StatusOK,
			jsonhttptest.WithExpectedResponse(mockWrappedCh.Data()[swarm.SpanSize:]),
			jsonhttptest.WithExpectedContentLength(len(mockWrappedCh.Data()[swarm.SpanSize:])),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
		)
	})

	t.Run("chunk wrapping", func(t *testing.T) {
		t.Parallel()

		testData := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}

		var (
			ch         = testingsoc.GenerateMockSOC(t, testData).Chunk()
			look       = newMockLookup(-1, 2, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", ""), http.StatusOK,
			jsonhttptest.WithExpectedResponse(testData),
			jsonhttptest.WithExpectedContentLength(len(testData)),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
		)
	})

	t.Run("legacy payload with non existing wrapped chunk", func(t *testing.T) {
		t.Parallel()

		wrappedRef := make([]byte, swarm.HashSize)
		_ = copy(wrappedRef, mockWrappedCh.Address().Bytes())
		wrappedRef[0]++

		var (
			ch      = toChunk(t, uint64(12121212), wrappedRef)
			look    = newMockLookup(-1, 2, ch, nil, &id{}, &id{})
			factory = newMockFactory(look)

			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", ""), http.StatusNotFound)
	})

	t.Run("bigger payload than one chunk", func(t *testing.T) {
		t.Parallel()

		testDataLen := 5000
		testData := testutil.RandBytesWithSeed(t, testDataLen, 1)
		s := splitter.NewSimpleSplitter(putter)
		addr, err := s.Split(context.Background(), io.NopCloser(bytes.NewReader(testData)), int64(testDataLen), false)
		if err != nil {
			t.Fatal(err)
		}

		// get root ch addr then add wrap it with soc
		testRootCh, err := mockStorer.ChunkStore().Get(context.Background(), addr)
		if err != nil {
			t.Fatal(err)
		}
		var (
			ch         = testingsoc.GenerateMockSOCWithSpan(t, testRootCh.Data()).Chunk()
			look       = newMockLookup(-1, 2, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		t.Run("retrieve chunk tree", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", ""), http.StatusOK,
				jsonhttptest.WithExpectedResponse(testData),
				jsonhttptest.WithExpectedContentLength(testDataLen),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			)
		})

		t.Run("retrieve only wrapped chunk", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", ""), http.StatusOK,
				jsonhttptest.WithRequestHeader(api.SwarmOnlyRootChunk, "true"),
				jsonhttptest.WithExpectedResponse(testRootCh.Data()),
				jsonhttptest.WithExpectedContentLength(len(testRootCh.Data())),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			)
		})
	})
}

// nolint:paralleltest
func TestFeed_Post(t *testing.T) {
	// post to owner, tpoic, then expect a reference
	// get the reference from the store, unmarshal to a
	// manifest entry and make sure all metadata correct
	var (
		logger          = log.Noop
		topic           = "aabbcc"
		mp              = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		mockStorer      = mockstorer.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Logger: logger,
			Post:   mp,
		})
		url = fmt.Sprintf("/feeds/%s/%s?type=%s", ownerString, topic, "sequence")
	)
	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
			jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
			jsonhttptest.WithExpectedJSONResponse(api.FeedReferenceResponse{
				Reference: expReference,
			}),
		)

		ls := loadsave.NewReadonly(mockStorer.ChunkStore())
		i, err := manifest.NewMantarayManifestReference(expReference, ls)
		if err != nil {
			t.Fatal(err)
		}
		e, err := i.Lookup(context.Background(), "/")
		if err != nil {
			t.Fatal(err)
		}

		meta := e.Metadata()
		if e := meta[api.FeedMetadataEntryOwner]; e != ownerString {
			t.Fatalf("owner mismatch. got %s want %s", e, ownerString)
		}
		if e := meta[api.FeedMetadataEntryTopic]; e != topic {
			t.Fatalf("topic mismatch. got %s want %s", e, topic)
		}
		if e := meta[api.FeedMetadataEntryType]; e != "Sequence" {
			t.Fatalf("type mismatch. got %s want %s", e, "Sequence")
		}
	})
	t.Run("postage", func(t *testing.T) {
		t.Run("err - bad batch", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchInvalid)
			jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusNotFound,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Message: "batch with id not found",
					Code:    http.StatusNotFound,
				}))
		})

		t.Run("ok - batch zeros", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchOk)
			jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusCreated,
				jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "true"),
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			)
		})
		t.Run("bad request - batch empty", func(t *testing.T) {
			hexbatch := hex.EncodeToString(batchEmpty)
			jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusBadRequest,
				jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, hexbatch),
			)
		})
	})
}

// TestDirectUploadFeed tests that the direct upload endpoint give correct error message in dev mode
func TestFeedDirectUpload(t *testing.T) {
	t.Parallel()
	var (
		topic           = "aabbcc"
		mp              = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		mockStorer      = mockstorer.New()
		client, _, _, _ = newTestServer(t, testServerOptions{
			Storer:  mockStorer,
			Post:    mp,
			BeeMode: api.DevMode,
		})
		url = fmt.Sprintf("/feeds/%s/%s?type=%s", ownerString, topic, "sequence")
	)
	jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusBadRequest,
		jsonhttptest.WithRequestHeader(api.SwarmDeferredUploadHeader, "false"),
		jsonhttptest.WithRequestHeader(api.SwarmPostageBatchIdHeader, batchOkStr),
		jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
			Message: api.ErrUnsupportedDevNodeOperation.Error(),
			Code:    http.StatusBadRequest,
		}),
	)
}

type factoryMock struct {
	sequenceCalled bool
	epochCalled    bool
	feed           *feeds.Feed
	lookup         feeds.Lookup
}

func newMockFactory(mockLookup feeds.Lookup) *factoryMock {
	return &factoryMock{lookup: mockLookup}
}

func (f *factoryMock) NewLookup(t feeds.Type, feed *feeds.Feed) (feeds.Lookup, error) {
	switch t {
	case feeds.Sequence:
		f.sequenceCalled = true
	case feeds.Epoch:
		f.epochCalled = true
	}
	f.feed = feed
	return f.lookup, nil
}

type mockLookup struct {
	at        int64
	after     uint64
	chunk     swarm.Chunk
	err       error
	cur, next feeds.Index
}

func newMockLookup(at int64, after uint64, ch swarm.Chunk, err error, cur, next feeds.Index) *mockLookup {
	return &mockLookup{at: at, after: after, chunk: ch, err: err, cur: cur, next: next}
}

func (l *mockLookup) At(_ context.Context, at int64, after uint64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	if l.at == -1 {
		// shortcut to ignore the value in the call since time.Now() is a moving target
		return l.chunk, l.cur, l.next, nil
	}
	if at == l.at && after == l.after {
		return l.chunk, l.cur, l.next, nil
	}
	return nil, nil, nil, errors.New("no feed update found")
}

func toChunk(t *testing.T, at uint64, payload []byte) swarm.Chunk {
	t.Helper()

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, at)
	content := append(ts, payload...)

	s := testingsoc.GenerateMockSOC(t, content)
	return s.Chunk()
}

type id struct{}

func (i *id) MarshalBinary() ([]byte, error) {
	return []byte("accd"), nil
}

func (i *id) String() string {
	return "44237"
}

func (*id) Next(last int64, at uint64) feeds.Index {
	return &id{}
}
