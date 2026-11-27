// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"testing"

	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

func tagsWithIdResource(id uint64) string { return fmt.Sprintf("/tags/%d", id) }

// failingListStorer makes ListSessions return err, as a canceled or timed out
// listing would.
type failingListStorer struct {
	api.Storer
	err error
}

func (s failingListStorer) ListSessions(context.Context, uint64, int, int) ([]storer.SessionInfo, error) {
	return nil, s.err
}

func TestListTagsContextErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "canceled", err: fmt.Errorf("list sessions: %w", context.Canceled), wantStatus: http.StatusRequestTimeout},
		{name: "deadline exceeded", err: fmt.Errorf("list sessions: %w", context.DeadlineExceeded), wantStatus: http.StatusServiceUnavailable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, _, _, _, _ := newTestServer(t, testServerOptions{
				Storer: failingListStorer{Storer: mockstorer.New(), err: tc.err},
			})

			jsonhttptest.Request(t, client, http.MethodGet, "/tags", tc.wantStatus)
		})
	}
}

func TestListTagsPaging(t *testing.T) {
	t.Parallel()

	var (
		tagsResource       = "/tags"
		client, _, _, _, _ = newTestServer(t, testServerOptions{
			Storer: mockstorer.New(),
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	for range 12 {
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
		)
	}

	// The listing is in lexicographic order of the decimal tag IDs: 1, 10, 11, 12, 2, ...
	want := []uint64{1, 10, 11, 12, 2, 3, 4, 5, 6, 7, 8, 9}

	uids := func(tags []api.TagResponse) []uint64 {
		out := make([]uint64, 0, len(tags))
		for _, tag := range tags {
			out = append(out, tag.Uid)
		}
		return out
	}

	t.Run("default page", func(t *testing.T) {
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		if diff := cmp.Diff(want, uids(resp.Tags)); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})

	t.Run("offset and limit", func(t *testing.T) {
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?offset=1&limit=3", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		if diff := cmp.Diff(want[1:4], uids(resp.Tags)); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})

	t.Run("after", func(t *testing.T) {
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?after=12&limit=3", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		if diff := cmp.Diff(want[4:7], uids(resp.Tags)); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})

	t.Run("after and offset", func(t *testing.T) {
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?after=12&offset=1&limit=2", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		if diff := cmp.Diff(want[5:7], uids(resp.Tags)); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})

	t.Run("invalid after", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?after=a", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid query params",
				Reasons: []jsonhttp.Reason{{Field: "after", Error: strconv.ErrSyntax.Error()}},
			}),
		)
	})

	t.Run("zero limit", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?limit=0", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListTagsResponse{
				Tags: []api.TagResponse{},
			}),
		)
	})

	t.Run("limit above the cap", func(t *testing.T) {
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?limit=5000", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)
		if diff := cmp.Diff(want, uids(resp.Tags)); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})
}

// nolint:paralleltest
func TestTags(t *testing.T) {
	var (
		tagsResource       = "/tags"
		storerMock         = mockstorer.New()
		client, _, _, _, _ = newTestServer(t, testServerOptions{
			Storer: storerMock,
			Post:   mockpost.New(mockpost.WithAcceptAll()),
		})
	)

	// list tags without anything pinned
	t.Run("list tags zero", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(api.ListTagsResponse{
				Tags: []api.TagResponse{},
			}),
		)
	})

	t.Run("list tags negative offset", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?offset=-1", http.StatusBadRequest)
	})

	t.Run("list tags negative limit", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource+"?limit=-1", http.StatusBadRequest)
	})

	t.Run("create tag", func(t *testing.T) {
		tr := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tr),
		)
	})

	t.Run("get non-existent tag", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(uint64(333)), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("list tags", func(t *testing.T) {
		// list all current tags
		var resp api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&resp),
		)

		// create 2 new tags
		tRes1 := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes1),
		)

		tRes2 := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes2),
		)

		resp.Tags = append(resp.Tags, tRes1, tRes2)

		// check if listing returns expected tags
		var got api.ListTagsResponse
		jsonhttptest.Request(t, client, http.MethodGet, tagsResource, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)

		sort.Slice(resp.Tags, func(i, j int) bool { return resp.Tags[i].Uid < resp.Tags[j].Uid })
		sort.Slice(got.Tags, func(i, j int) bool { return got.Tags[i].Uid < got.Tags[j].Uid })

		if diff := cmp.Diff(resp.Tags, got.Tags); diff != "" {
			t.Fatalf("unexpected tags (-want +have):\n%s", diff)
		}
	})

	t.Run("delete non-existent tag", func(t *testing.T) {
		// try to delete non-existent tag
		jsonhttptest.Request(t, client, http.MethodDelete, tagsWithIdResource(uint64(333)), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("delete tag", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{}),
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		// check tag existence
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tRes.Uid), http.StatusOK)

		// delete tag through API
		jsonhttptest.Request(t, client, http.MethodDelete, tagsWithIdResource(tRes.Uid), http.StatusNoContent,
			jsonhttptest.WithNoResponseBody(),
		)

		// try to get tag
		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tRes.Uid), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("done split non-existent tag", func(t *testing.T) {
		// non-existent tag
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(uint64(333)), http.StatusNotFound,
			jsonhttptest.WithJSONRequestBody(api.TagResponse{}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "tag not present",
				Code:    http.StatusNotFound,
			}),
		)
	})

	t.Run("done split", func(t *testing.T) {
		// create a tag through API
		tRes := api.TagResponse{}
		jsonhttptest.Request(t, client, http.MethodPost, tagsResource, http.StatusCreated,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)
		tagId := tRes.Uid

		// generate address to be supplied to the done split
		addr := swarm.RandAddress(t)

		// call done split
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{
				Address: addr,
			}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "ok",
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		if !tRes.Address.Equal(addr) {
			t.Fatalf("invalid session reference got %s want %s", tRes.Address, addr)
		}

		// try different address value
		addr = swarm.RandAddress(t)

		// call done split
		jsonhttptest.Request(t, client, http.MethodPatch, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithJSONRequestBody(api.TagRequest{
				Address: addr,
			}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "ok",
				Code:    http.StatusOK,
			}),
		)

		jsonhttptest.Request(t, client, http.MethodGet, tagsWithIdResource(tagId), http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&tRes),
		)

		if !tRes.Address.Equal(addr) {
			t.Fatalf("invalid session reference got %s want %s", tRes.Address, addr)
		}
	})
}

func TestTagsHandlersInvalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name  string
		tagID string
		want  jsonhttp.StatusResponse
	}{{
		name:  "id - invalid value",
		tagID: "a",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "id",
					Error: strconv.ErrSyntax.Error(),
				},
			},
		},
	}}

	for _, method := range []string{http.MethodGet, http.MethodDelete, http.MethodPatch} {
		for _, tc := range tests {
			t.Run(method+" "+tc.name, func(t *testing.T) {
				t.Parallel()

				jsonhttptest.Request(t, client, method, "/tags/"+tc.tagID, tc.want.Code,
					jsonhttptest.WithExpectedJSONResponse(tc.want),
				)
			})
		}
	}
}

// isTagFoundInResponse verifies that the tag id is found in the supplied HTTP headers
// if an API tag response is supplied, it also verifies that it contains an id which matches the headers
func isTagFoundInResponse(t *testing.T, headers http.Header, tr *api.TagResponse) uint64 {
	t.Helper()

	idStr := headers.Get(api.SwarmTagHeader)
	if idStr == "" {
		t.Fatalf("could not find tag id header in chunk upload response")
	}
	nId, err := strconv.Atoi(idStr)
	id := uint64(nId)
	if err != nil {
		t.Fatal(err)
	}
	if tr != nil {
		if id != tr.Uid {
			t.Fatalf("expected created tag id to be %d, but got %d when uploading chunk", tr.Uid, id)
		}
	}
	return id
}
