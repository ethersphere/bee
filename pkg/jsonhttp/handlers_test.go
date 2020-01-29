// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttp_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

func TestMethodHandler(t *testing.T) {
	h := jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, err := ioutil.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprint(w, "got: ", string(got))
		}),
	}

	t.Run("method allowed", func(t *testing.T) {
		body := "test body"

		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		w := httptest.NewRecorder()

		h.ServeHTTP(w, r)

		statusCode := w.Result().StatusCode
		if statusCode != http.StatusOK {
			t.Errorf("got status code %d, want %d", statusCode, http.StatusOK)
		}

		wantBody := "got: " + body
		gotBody := w.Body.String()

		if gotBody != wantBody {
			t.Errorf("got body %q, want %q", gotBody, wantBody)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		h.ServeHTTP(w, r)

		statusCode := w.Result().StatusCode
		wantCode := http.StatusMethodNotAllowed
		if statusCode != wantCode {
			t.Errorf("got status code %d, want %d", statusCode, wantCode)
		}

		var m *jsonhttp.StatusResponse

		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Errorf("json unmarshal response body: %s", err)
		}

		if m.Code != wantCode {
			t.Errorf("got message code %d, want %d", m.Code, wantCode)
		}

		wantMessage := http.StatusText(wantCode)
		if m.Message != wantMessage {
			t.Errorf("got message message %q, want %q", m.Message, wantMessage)
		}
	})
}

func TestNotFoundHandler(t *testing.T) {
	w := httptest.NewRecorder()

	jsonhttp.NotFoundHandler(w, nil)

	statusCode := w.Result().StatusCode
	wantCode := http.StatusNotFound
	if statusCode != wantCode {
		t.Errorf("got status code %d, want %d", statusCode, wantCode)
	}

	var m *jsonhttp.StatusResponse

	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Errorf("json unmarshal response body: %s", err)
	}

	if m.Code != wantCode {
		t.Errorf("got message code %d, want %d", m.Code, wantCode)
	}

	wantMessage := http.StatusText(wantCode)
	if m.Message != wantMessage {
		t.Errorf("got message message %q, want %q", m.Message, wantMessage)
	}
}
