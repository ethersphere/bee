// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/storage/mock"
	"github.com/ethersphere/bee/pkg/storage/mock/validator"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

func TestTags(t *testing.T) {
	var (
		resource             = func(addr swarm.Address) string { return "/bzz-chunk/" + addr.String() }
		tagResource          = func(addr swarm.Address) string { return "/bzz-tag/addr/" + addr.String() }
		tagResourceUUid      = func(uuid uint64) string { return "/bzz-tag/uuid/" + strconv.FormatUint(uuid, 10) }
		validHash            = swarm.MustParseHexAddress("aabbcc")
		validContent         = []byte("bbaatt")
		mockValidator        = validator.NewMockValidator(validHash, validContent)
		mockValidatingStorer = mock.NewValidatingStorer(mockValidator)
		tag                  = tags.NewTags()
		client, cleanup      = newTestServer(t, testServerOptions{
			Storer: mockValidatingStorer,
			Tags:   tag,
		})
	)
	defer cleanup()

	t.Run("tag-header-in-return", func(t *testing.T) {
		headers := make(map[string][]string)
		headers[api.TagHeaderName] = []string{"2341312131"} // the value doesn't matter, it is random so we cant test for that
		rcvdHeaders := jsonhttptest.ResponseDirectReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		found := false
		for k, v := range rcvdHeaders {
			if api.TagHeaderName == strings.ToLower(k) {
				found = true
				_, err := strconv.ParseUint(v[0], 10, 32)
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		if !found {
			t.Fatalf("could not find tagid header in response")
		}
	})

	t.Run("get-tag-using-address", func(t *testing.T) {
		headers := make(map[string][]string)
		headers[api.TagHeaderName] = []string{"2341312131"} // the value doesn't matter, it is random so we cant test for that
		rcvdHeaders := jsonhttptest.ResponseDirectReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		found := false
		var uuid uint64
		for k, v := range rcvdHeaders {
			if api.TagHeaderName == strings.ToLower(k) {
				found = true
				uid, err := strconv.ParseUint(v[0], 10, 32)
				if err != nil {
					t.Fatal(err)
				}
				uuid = uid
			}
		}

		if !found {
			t.Fatalf("could not find tagid header in response")
		}

		// Request the tag and see of the UUID is the same
		resp := request(t, client, http.MethodGet, tagResource(validHash), nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		ta := &tags.Tag{}
		err = json.Unmarshal(data, &ta)
		if err != nil {
			t.Fatalf("could nor unmarshal response")
		}

		if uuid != uint64(ta.Uid) {
			t.Fatalf("Invalid uuid response")
		}

	})

	t.Run("get-tag-using-uuid", func(t *testing.T) {
		headers := make(map[string][]string)
		headers[api.TagHeaderName] = []string{"2341312131"} // the value doesn't matter, it is random so we cant test for that
		rcvdHeaders := jsonhttptest.ResponseDirectReceiveHeaders(t, client, http.MethodPost, resource(validHash), bytes.NewReader(validContent), http.StatusOK, jsonhttp.StatusResponse{
			Message: http.StatusText(http.StatusOK),
			Code:    http.StatusOK,
		})

		found := false
		var uuid uint64
		for k, v := range rcvdHeaders {
			if api.TagHeaderName == strings.ToLower(k) {
				found = true
				uid, err := strconv.ParseUint(v[0], 10, 32)
				if err != nil {
					t.Fatal(err)
				}
				uuid = uid
			}
		}

		if !found {
			t.Fatalf("could not find tagid header in response")
		}

		// Request the tag and see of the UUID is the same
		resp := request(t, client, http.MethodGet, tagResourceUUid(uuid), nil, http.StatusOK)
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		ta := &tags.Tag{}
		err = json.Unmarshal(data, &ta)
		if err != nil {
			t.Fatalf("could nor unmarshal response")
		}

		if uuid != uint64(ta.Uid) {
			t.Fatalf("Invalid uuid response")
		}

	})
}
