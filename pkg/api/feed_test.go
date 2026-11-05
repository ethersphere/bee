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
	"net/url"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/feeds"
	"github.com/ethersphere/bee/v2/pkg/feeds/sequence"
	"github.com/ethersphere/bee/v2/pkg/file/loadsave"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/file/splitter"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/manifest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	testingsoc "github.com/ethersphere/bee/v2/pkg/soc/testing"
	"github.com/ethersphere/bee/v2/pkg/storage"
	testingc "github.com/ethersphere/bee/v2/pkg/storage/testing"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

const ownerString = "8d3766440f0d7b949a5e32995d09619a7f86e632"

var expReference = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")

func TestFeed_Get(t *testing.T) {
	t.Parallel()

	var (
		feedResource = func(owner, topic string, query ...string) string {
			values := url.Values{}
			for i := 0; i+1 < len(query); i += 2 {
				values.Set(query[i], query[i+1])
			}

			baseURL := fmt.Sprintf("/feeds/%s/%s", owner, topic)
			if len(values) > 0 {
				return baseURL + "?" + values.Encode()
			}
			return baseURL
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

	t.Run("with after", func(t *testing.T) {
		t.Parallel()

		var (
			timestamp          = int64(12121212)
			ch                 = toChunk(t, uint64(timestamp), mockWrappedCh.Address().Bytes())
			look               = newMockLookup(3, ch, nil, &id{}, &id{})
			factory            = newMockFactory(look)
			idBytes, _         = (&id{}).MarshalBinary()
			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", "after", "3"), http.StatusOK,
			jsonhttptest.WithExpectedResponse(mockWrappedCh.Data()[swarm.SpanSize:]),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexNextHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v1"),
		)
	})

	t.Run("with index", func(t *testing.T) {
		t.Parallel()

		var (
			indexStorer        = mockstorer.New()
			factory            = newMockFactory(newMockLookup(0, nil, nil, nil, nil))
			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: indexStorer,
				Feeds:  factory,
			})
		)

		indexPutter, err := indexStorer.Upload(context.Background(), false, 0)
		if err != nil {
			t.Fatal(err)
		}
		key, err := crypto.GenerateSecp256k1Key()
		if err != nil {
			t.Fatal(err)
		}
		topic := []byte{0xaa, 0xbb, 0xcc}
		updater, err := sequence.NewUpdater(indexPutter, crypto.NewDefaultSigner(key), topic)
		if err != nil {
			t.Fatal(err)
		}
		payloads := [][]byte{[]byte("update 0"), []byte("update 1"), []byte("update 2")}
		for _, p := range payloads {
			if err := updater.Update(context.Background(), 0, p); err != nil {
				t.Fatal(err)
			}
		}
		owner := hex.EncodeToString(updater.Feed().Owner.Bytes())

		for i, p := range payloads {
			idxBytes, err := sequence.NewIndex(uint64(i)).MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(owner, "aabbcc", "index", strconv.Itoa(i)), http.StatusOK,
				jsonhttptest.WithExpectedResponse(p),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idxBytes)),
				jsonhttptest.WithNonEmptyResponseHeader(api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v2"),
			)
		}

		t.Run("next index header is omitted", func(t *testing.T) {
			resp := jsonhttptest.Request(t, client, http.MethodGet, feedResource(owner, "aabbcc", "index", "1"), http.StatusOK)
			if v := resp.Get(api.SwarmFeedIndexNextHeader); v != "" {
				t.Fatalf("got %s header %q, want none", api.SwarmFeedIndexNextHeader, v)
			}
		})

		t.Run("index not found", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(owner, "aabbcc", "index", "3"), http.StatusNotFound,
				jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
					Code:    http.StatusNotFound,
					Message: "no update found",
				}),
			)
		})

		t.Run("invalid index", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(owner, "aabbcc", "index", "abc"), http.StatusBadRequest)
		})

		t.Run("index takes precedence over after", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(owner, "aabbcc", "index", "1", "after", "2"), http.StatusOK,
				jsonhttptest.WithExpectedResponse(payloads[1]),
			)
		})

		if factory.sequenceCalled {
			t.Fatal("feed lookup must not be used when an index is given")
		}
	})

	t.Run("with index retrieval errors", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name    string
			err     error
			code    int
			message string
		}{
			{"not found", storage.ErrNotFound, http.StatusNotFound, "no update found"},
			{"no peers", topology.ErrNotFound, http.StatusServiceUnavailable, "feed update could not be retrieved because no usable peer was available"},
			{"other error", errors.New("disk on fire"), http.StatusInternalServerError, "get feed update failed"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				client, _, _, _, _ := newTestServer(t, testServerOptions{
					Storer: errStorer{Storer: mockstorer.New(), err: fmt.Errorf("retrieve: %w", tc.err)},
					Feeds:  newMockFactory(newMockLookup(0, nil, nil, nil, nil)),
				})
				jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", "index", "0"), tc.code,
					jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
						Code:    tc.code,
						Message: tc.message,
					}),
				)
			})
		}
	})

	t.Run("latest with legacy payload", func(t *testing.T) {
		t.Parallel()

		var (
			timestamp  = int64(12121212)
			ch         = toChunk(t, uint64(timestamp), mockWrappedCh.Address().Bytes())
			look       = newMockLookup(0, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusOK,
			jsonhttptest.WithExpectedResponse(mockWrappedCh.Data()[swarm.SpanSize:]),
			jsonhttptest.WithExpectedContentLength(len(mockWrappedCh.Data()[swarm.SpanSize:])),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v1"),
		)
	})

	t.Run("chunk wrapping", func(t *testing.T) {
		t.Parallel()

		testData := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8}

		var (
			ch         = testingsoc.GenerateMockSOC(t, testData).Chunk()
			look       = newMockLookup(0, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusOK,
			jsonhttptest.WithExpectedResponse(testData),
			jsonhttptest.WithExpectedContentLength(len(testData)),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
			jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
			jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
			jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v2"),
		)
	})

	t.Run("legacy payload with non existing wrapped chunk", func(t *testing.T) {
		t.Skip()
		/*
			This test has been disabled since it cannot be supported with the automatic
			Feed resolution logic that is now in place. In case automatic feed resolution
			would be removed at some point, this test can be reactived. The issue is
			thoroughly described in the PR: https://github.com/ethersphere/bee/pull/5287
		*/

		wrappedRef := make([]byte, swarm.HashSize)
		_ = copy(wrappedRef, mockWrappedCh.Address().Bytes())
		wrappedRef[0]++

		var (
			ch      = toChunk(t, uint64(12121212), wrappedRef)
			look    = newMockLookup(0, ch, nil, &id{}, &id{})
			factory = newMockFactory(look)

			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusNotFound)
	})

	t.Run("query parameter legacy feed resolve", func(t *testing.T) {
		t.Parallel()

		var (
			look               = newMockLookup(0, nil, errors.New("dummy"), &id{}, &id{})
			factory            = newMockFactory(look)
			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		// Test with the legacyFeed parameter set to true which should add the query parameter
		jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusNotFound)
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
			look       = newMockLookup(0, ch, nil, &id{}, &id{})
			factory    = newMockFactory(look)
			idBytes, _ = (&id{}).MarshalBinary()

			client, _, _, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Feeds:  factory,
			})
		)

		t.Run("retrieve chunk tree", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusOK,
				jsonhttptest.WithExpectedResponse(testData),
				jsonhttptest.WithExpectedContentLength(testDataLen),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.ContentDispositionHeader),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v2"),
			)
		})

		t.Run("retrieve only wrapped chunk", func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc"), http.StatusOK,
				jsonhttptest.WithRequestHeader(api.SwarmOnlyRootChunk, "true"),
				jsonhttptest.WithExpectedResponse(testRootCh.Data()),
				jsonhttptest.WithExpectedContentLength(len(testRootCh.Data())),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedIndexHeader, hex.EncodeToString(idBytes)),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmFeedIndexNextHeader),
				jsonhttptest.WithExpectedResponseHeader(api.AccessControlExposeHeaders, api.SwarmSocSignatureHeader),
				jsonhttptest.WithExpectedResponseHeader(api.ContentTypeHeader, "application/octet-stream"),
				jsonhttptest.WithExpectedResponseHeader(api.SwarmFeedResolvedVersionHeader, "v2"),
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
		logger             = log.Noop
		topic              = "aabbcc"
		mp                 = mockpost.New(mockpost.WithIssuer(postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)))
		mockStorer         = mockstorer.New()
		client, _, _, _, _ = newTestServer(t, testServerOptions{
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

		ls := loadsave.NewReadonly(mockStorer.ChunkStore(), mockStorer.Cache(), redundancy.DefaultDownloadLevel)
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

type factoryMock struct {
	sequenceCalled bool
	feed           *feeds.Feed
	lookup         feeds.Lookup
}

func newMockFactory(mockLookup feeds.Lookup) *factoryMock {
	return &factoryMock{lookup: mockLookup}
}

func (f *factoryMock) NewLookup(t feeds.Type, feed *feeds.Feed) (feeds.Lookup, error) {
	if t == feeds.Sequence {
		f.sequenceCalled = true
	}
	f.feed = feed
	return f.lookup, nil
}

// mockLookup returns its update only when called with the expected after index.
type mockLookup struct {
	after     uint64
	chunk     swarm.Chunk
	err       error
	cur, next feeds.Index
}

func newMockLookup(after uint64, ch swarm.Chunk, err error, cur, next feeds.Index) *mockLookup {
	return &mockLookup{after: after, chunk: ch, err: err, cur: cur, next: next}
}

func (l *mockLookup) At(_ context.Context, _ int64, after uint64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	if after != l.after {
		return nil, nil, nil, errors.New("no feed update found")
	}
	return l.chunk, l.cur, l.next, l.err
}

// errStorer is an api.Storer whose downloads always fail with err.
type errStorer struct {
	api.Storer
	err error
}

func (s errStorer) Download(bool) storage.Getter { return errGetter{err: s.err} }

type errGetter struct{ err error }

func (g errGetter) Get(context.Context, swarm.Address) (swarm.Chunk, error) { return nil, g.err }

func toChunk(t *testing.T, at uint64, payload []byte) swarm.Chunk {
	t.Helper()

	ts := make([]byte, 8, 8+len(payload))
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
