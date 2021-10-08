// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttp_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

func TestMethodHandler(t *testing.T) {
	contentType := "application/swarm"

	h := jsonhttp.MethodHandler{
		"POST": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", contentType)
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

		if got := w.Header().Get("Content-Type"); got != contentType {
			t.Errorf("got content type %q, want %q", got, contentType)
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

		testContentType(t, w)
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

	testContentType(t, w)
}

func TestNewMaxBodyBytesHandler(t *testing.T) {
	var limit int64 = 10

	h := jsonhttp.NewMaxBodyBytesHandler(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			if jsonhttp.HandleBodyReadError(err, w) {
				return
			}
			jsonhttp.InternalServerError(w, nil)
			return
		}
		jsonhttp.OK(w, nil)
	}))

	for _, tc := range []struct {
		name                 string
		body                 string
		withoutContentLength bool
		wantCode             int
	}{
		{
			name:     "empty",
			wantCode: http.StatusOK,
		},
		{
			name:                 "within limit without content length header",
			body:                 "data",
			withoutContentLength: true,
			wantCode:             http.StatusOK,
		},
		{
			name:     "within limit",
			body:     "data",
			wantCode: http.StatusOK,
		},
		{
			name:     "over limit",
			body:     "long test data",
			wantCode: http.StatusRequestEntityTooLarge,
		},
		{
			name:                 "over limit without content length header",
			body:                 "long test data",
			withoutContentLength: true,
			wantCode:             http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.withoutContentLength {
				r.Header.Del("Content-Length")
				r.ContentLength = 0
			}
			w := httptest.NewRecorder()

			h.ServeHTTP(w, r)

			if w.Code != tc.wantCode {
				t.Errorf("got http response code %d, want %d", w.Code, tc.wantCode)
			}

			var m *jsonhttp.StatusResponse

			if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
				t.Errorf("json unmarshal response body: %s", err)
			}

			if m.Code != tc.wantCode {
				t.Errorf("got message code %d, want %d", m.Code, tc.wantCode)
			}

			wantMessage := http.StatusText(tc.wantCode)
			if m.Message != wantMessage {
				t.Errorf("got message message %q, want %q", m.Message, wantMessage)
			}
		})
	}
}
