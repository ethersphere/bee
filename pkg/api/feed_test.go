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
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/feeds"
	"github.com/ethersphere/bee/pkg/file/loadsave"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/manifest"
	"github.com/ethersphere/bee/pkg/soc"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

const ownerString = "8d3766440f0d7b949a5e32995d09619a7f86e632"

func TestFeed_Get(t *testing.T) {
	var (
		feedResource = func(owner, topic, feedType, at string) string {
			if feedType != "" {
				return fmt.Sprintf("/feeds/%s/%s?type=%s", owner, topic, feedType)
			}
			if at != "" {
				return fmt.Sprintf("/feeds/%s/%s?at=%s", owner, topic, at)
			}
			return fmt.Sprintf("/feeds/%s/%s", owner, topic)
		}
		_              = testingc.GenerateTestRandomChunk()
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)
		_              = common.HexToAddress(ownerString)
		mockStorer     = mock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
		})
	)

	t.Run("malformed owner", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, feedResource("xyz", "cc", "", ""), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad owner",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed topic", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, feedResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "xxzzyy", "", ""), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "bad topic",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("bad type", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, feedResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "aabbcc", "unbekannt", ""), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "unknown type",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	//t.Run("at malformed", func(t *testing.T) {
	//t.Fail("todo")
	//jsonhttptest.Request(t, client, http.MethodGet, feedResource("8d3766440f0d7b949a5e32995d09619a7f86e632", "aabbcc", "unbekannt"), http.StatusBadRequest,
	//jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
	//Message: "unknown type",
	//Code:    http.StatusBadRequest,
	//}),
	//)
	//})

	t.Run("with at", func(t *testing.T) {
		var (
			timestamp      = int64(12121212)
			timestampBytes = make([]byte, 8)
			expReference   = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")
			ch             = mockUpdate(timestamp, expReference)
			look           = newMockLookup(12, 0, ch, nil, nil)
			factory        = newMockFactory(look)

			client, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Tags:   tag,
				Feeds:  factory,
			})
		)

		binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))

		respHeaders := jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", "", "12"), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.FeedReferenceResponse{Reference: expReference}),
		)

		if h := respHeaders[api.SwarmFeedIndexHeader]; len(h) > 0 {
			b, err := hex.DecodeString(h[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b, timestampBytes) {
				t.Fatalf("feed index header mismatch. got %v want %v", b, timestampBytes)
			}
		} else {
			t.Fatal("expected swarm feed index header to be set")
		}
	})

	t.Run("latest", func(t *testing.T) {
		var (
			timestamp      = int64(12121212)
			timestampBytes = make([]byte, 8)
			expReference   = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")
			ch             = mockUpdate(timestamp, expReference)
			_              = common.HexToAddress(ownerString)
			look           = newMockLookup(-1, 2, ch, nil, nil)
			factory        = newMockFactory(look)

			client, _, _ = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Tags:   tag,
				Feeds:  factory,
			})
		)

		binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))

		respHeaders := jsonhttptest.Request(t, client, http.MethodGet, feedResource(ownerString, "aabbcc", "", ""), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.FeedReferenceResponse{Reference: expReference}),
		)

		if h := respHeaders[api.SwarmFeedIndexHeader]; len(h) > 0 {
			b, err := hex.DecodeString(h[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b, timestampBytes) {
				t.Fatalf("feed index header mismatch. got %v want %v", b, timestampBytes)
			}
		} else {
			t.Fatal("expected swarm feed index header to be set")
		}
	})
}

func TestFeed_Post(t *testing.T) {
	// post to owner, tpoic, then expect a reference
	// get the reference from the store, unmarshal to a
	// manifest entry and make sure all metadata correct
	var (
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)
		topic          = "aabbcc"
		expRef         = swarm.MustParseHexAddress("891a1d1c8436c792d02fc2e8883fef7ab387eaeaacd25aa9f518be7be7856d54")
		mockStorer     = mock.NewStorer()
		client, _, _   = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tag,
			Logger: logger,
		})
	)

	t.Run("ok", func(t *testing.T) {
		url := fmt.Sprintf("/feeds/%s/%s?type=%s", ownerString, topic, "sequence")
		jsonhttptest.Request(t, client, http.MethodPost, url, http.StatusCreated,
			jsonhttptest.WithExpectedJSONResponse(api.FeedReferenceResponse{
				Reference: expRef,
			}),
		)

		ls := loadsave.New(mockStorer, storage.ModePutUpload, false)
		i, err := manifest.NewMantarayManifestReference(expRef, ls)
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
}

func TestFeed_Put(t *testing.T) {
	var (
		_              = testingc.GenerateTestRandomChunk()
		mockStatestore = statestore.NewStateStore()
		logger         = logging.New(ioutil.Discard, 0)
		tag            = tags.NewTags(mockStatestore, logger)
		_              = common.HexToAddress(ownerString)
		mockStorer     = mock.NewStorer()
	)

	t.Run("update once", func(t *testing.T) {
		nextId := &id{}
		idB, _ := nextId.MarshalBinary()
		topic := "aabbcc"
		topicBytes, _ := hex.DecodeString(topic)
		th, _ := crypto.LegacyKeccak256(topicBytes)
		idBytes, _ := crypto.LegacyKeccak256(append(th, idB...))
		var (
			timestamp      = int64(12121212)
			timestampBytes = make([]byte, 8)
			ref            = swarm.MustParseHexAddress("accdaccd")
			ch, _          = toChunk(uint64(timestamp), ref.Bytes())
			sch, owner, _  = socChunk(t, idBytes, ch)
			ssch, _        = sch.ToChunk()
			look           = newMockLookup(-1, 2, ch, nil, nextId)
			factory        = newMockFactory(look)
			sigStr         = hex.EncodeToString(sch.Signature())
			client, _, _   = newTestServer(t, testServerOptions{
				Storer: mockStorer,
				Tags:   tag,
				Feeds:  factory,
			})
		)
		binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))
		url := fmt.Sprintf("/feeds/%s/%s/%s?sig=%s&at=%d", hex.EncodeToString(owner), topic, ref.String(), sigStr, timestamp)
		_ = jsonhttptest.Request(t, client, http.MethodPut, url, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.FeedReferenceResponse{Reference: ref}),
		)
		a, e := sch.Address()
		if e != nil {
			t.Fatal(e)
		}
		// find out if the soc made it into the localstore
		cch, err := mockStorer.Get(context.Background(), storage.ModeGetRequest, a)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(cch.Data(), ssch.Data()) {
			t.Fatal("data mismatch")
		}
	})
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

func (f *factoryMock) NewLookup(t feeds.Type, feed *feeds.Feed) feeds.Lookup {
	switch t {
	case feeds.Sequence:
		f.sequenceCalled = true
	case feeds.Epoch:
		f.epochCalled = true
	}
	f.feed = feed
	return f.lookup
}

type mockLookup struct {
	at, after int64
	chunk     swarm.Chunk
	err       error
	next      feeds.Index
}

func newMockLookup(at, after int64, ch swarm.Chunk, err error, next feeds.Index) *mockLookup {
	return &mockLookup{at: at, after: after, chunk: ch, err: err, next: next}
}

func (l *mockLookup) At(_ context.Context, at, after int64) (swarm.Chunk, feeds.Index, feeds.Index, error) {
	if l.at == -1 {
		// shortcut to ignore the value in the call since time.Now() is a moving target
		return l.chunk, nil, l.next, nil
	}
	if at == l.at && after == l.after {
		return l.chunk, nil, l.next, nil
	}
	return nil, nil, nil, errors.New("no feed update found")
}

func mockUpdate(t int64, a swarm.Address) swarm.Chunk {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	tb := make([]byte, swarm.SpanSize)
	binary.BigEndian.PutUint64(tb, uint64(t))
	tb = append(tb, a.Bytes()...)
	_, err := hasher.Write(tb)
	if err != nil {
		panic(err)
	}
	span := uint64(len(tb))
	b := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(b, span)
	err = hasher.SetSpanBytes(b)
	if err != nil {
		panic(err)
	}
	s := swarm.NewAddress(hasher.Sum(nil))
	return swarm.NewChunk(s, append(b, tb...))
}

func toChunk(at uint64, payload []byte) (swarm.Chunk, error) {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, at)
	content := append(ts, payload...)
	//return cac.New(content)
	_, err := hasher.Write(content)
	if err != nil {
		return nil, err
	}
	span := make([]byte, 8)
	binary.LittleEndian.PutUint64(span, uint64(len(content)))
	err = hasher.SetSpanBytes(span)
	if err != nil {
		return nil, err
	}
	return swarm.NewChunk(swarm.NewAddress(hasher.Sum(nil)), append(append([]byte{}, span...), content...)), nil
}

func socChunk(t *testing.T, id []byte, ch swarm.Chunk) (*soc.Soc, []byte, []byte) {
	// create a valid soc
	privKey, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(privKey)

	sch := soc.New(id, ch)
	if err != nil {
		t.Fatal(err)
	}
	err = sch.AddSigner(signer)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = sch.ToChunk()

	return sch, sch.OwnerAddress(), ch.Data()
}

type id struct {
}

func (i *id) MarshalBinary() ([]byte, error) {
	return []byte("accd"), nil
}
