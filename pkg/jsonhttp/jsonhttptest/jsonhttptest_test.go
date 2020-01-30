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

func TestResponse(t *testing.T) {
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

	t.Run("direct", func(t *testing.T) {
		jsonhttptest.ResponseDirect(t, c, wantMethod, s.URL+wantPath, strings.NewReader(wantBody), http.StatusCreated, response{
			Message: message,
		})

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

	t.Run("unmarshal", func(t *testing.T) {
		var r response
		jsonhttptest.ResponseUnmarshal(t, c, wantMethod, s.URL+wantPath, strings.NewReader(wantBody), http.StatusCreated, &r)

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
			t.Errorf("got messag %s, want %s", r.Message, message)
		}
	})
}
