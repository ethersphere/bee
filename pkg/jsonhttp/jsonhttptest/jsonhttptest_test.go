// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestRequest(t *testing.T) {
	type response struct {
		Message string `json:"message"`
	}
	message := "text"

	wantMethod, wantPath, wantBody := http.MethodPatch, "/testing", "request body"
	var gotMethod, gotPath, gotBody string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
		gotBody = string(b)
		jsonhttp.Created(w, response{
			Message: message,
		})
	}))
	defer s.Close()

	c := s.Client()

	t.Run("WithExpectedJSONResponse", func(t *testing.T) {
		jsonhttptest.Request(t, c, wantMethod, s.URL+wantPath, http.StatusCreated,
			jsonhttptest.WithRequestBody(strings.NewReader(wantBody)),
			jsonhttptest.WithExpectedJSONResponse(response{
				Message: message,
			}),
		)

		if gotMethod != wantMethod {
			t.Errorf("got method %s, want %s", gotMethod, wantMethod)
		}
		if gotPath != wantPath {
			t.Errorf("got path %s, want %s", gotPath, wantPath)
		}
		if gotBody != wantBody {
			t.Errorf("got body %s, want %s", gotBody, wantBody)
		}
	})

	t.Run("WithUnmarshalResponse", func(t *testing.T) {
		var r response
		jsonhttptest.Request(t, c, wantMethod, s.URL+wantPath, http.StatusCreated,
			jsonhttptest.WithRequestBody(strings.NewReader(wantBody)),
			jsonhttptest.WithUnmarshalResponse(&r),
		)

		if gotMethod != wantMethod {
			t.Errorf("got method %s, want %s", gotMethod, wantMethod)
		}
		if gotPath != wantPath {
			t.Errorf("got path %s, want %s", gotPath, wantPath)
		}
		if gotBody != wantBody {
			t.Errorf("got body %s, want %s", gotBody, wantBody)
		}
		if r.Message != message {
			t.Errorf("got message %s, want %s", r.Message, message)
		}
	})
}
