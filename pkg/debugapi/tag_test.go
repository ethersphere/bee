// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/debugapi"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/logging"
	statestore "github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/mock"
	testingc "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func tagsWithIdResource(id uint32) string { return fmt.Sprintf("/tags/%d", id) }

func TestTags(t *testing.T) {
	var (
		logger         = logging.New(ioutil.Discard, 0)
		chunk          = testingc.GenerateTestRandomChunk()
		mockStorer     = mock.NewStorer()
		mockStatestore = statestore.NewStateStore()
		tagsStore      = tags.NewTags(mockStatestore, logger)
		testServer     = newTestServer(t, testServerOptions{
			Storer: mockStorer,
			Tags:   tagsStore,
		})
	)

	_, err := mockStorer.Put(context.Background(), storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("all", func(t *testing.T) {
		tag, err := tagsStore.Create(0)
		if err != nil {
			t.Fatal(err)
		}

		_ = tag.Inc(tags.StateSplit)
		_ = tag.Inc(tags.StateStored)
		_ = tag.Inc(tags.StateSeen)
		_ = tag.Inc(tags.StateSent)
		_ = tag.Inc(tags.StateSynced)

		_, err = tag.DoneSplit(chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		tagValueTest(t, tag.Uid, 1, 1, 1, 1, 1, 1, chunk.Address(), testServer.Client)
	})
}

func tagValueTest(t *testing.T, id uint32, split, stored, seen, sent, synced, total int64, address swarm.Address, client *http.Client) {
	t.Helper()

	tag := debugapi.TagResponse{}
	jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(id), http.StatusOK,
		jsonhttptest.WithUnmarshalJSONResponse(&tag),
	)

	if tag.Split != split {
		t.Errorf("tag split count mismatch. got %d want %d", tag.Split, split)
	}
	if tag.Stored != stored {
		t.Errorf("tag stored count mismatch. got %d want %d", tag.Stored, stored)
	}
	if tag.Seen != seen {
		t.Errorf("tag seen count mismatch. got %d want %d", tag.Seen, seen)
	}
	if tag.Sent != sent {
		t.Errorf("tag sent count mismatch. got %d want %d", tag.Sent, sent)
	}
	if tag.Synced != synced {
		t.Errorf("tag synced count mismatch. got %d want %d", tag.Synced, synced)
	}
	if tag.Total != total {
		t.Errorf("tag total count mismatch. got %d want %d", tag.Total, total)
	}

	if !tag.Address.Equal(address) {
		t.Errorf("address mismatch: expected %s got %s", address.String(), tag.Address.String())
	}
}
