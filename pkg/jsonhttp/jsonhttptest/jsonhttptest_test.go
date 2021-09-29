// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
)

func TestRequest_statusCode(t *testing.T) {
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusBadRequest)
	})

	assert(t, `got response status 400 Bad Request, want 200 OK`, "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})
}

func TestRequest_method(t *testing.T) {
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusMethodNotAllowed)
	})
}

func TestRequest_url(t *testing.T) {
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint+"/bzz", http.StatusNotFound)
	})
}

func TestRequest_responseHeader(t *testing.T) {
	headerName := "Swarm-Header"
	headerValue := "somevalue"
	var gotValue string

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(headerName, headerValue)
	}))

	assert(t, "", "", func(m *mock) {
		gotValue = jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK).Get(headerName)
	})
	if gotValue != headerValue {
		t.Errorf("got header %q, want %q", gotValue, headerValue)
	}
}

func TestWithContext(t *testing.T) {
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel the context to detect the fatal error

	assert(t, "", fmt.Sprintf("Get %q: context canceled", endpoint), func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithContext(ctx),
		)
	})
}

func TestWithRequestBody(t *testing.T) {
	wantBody := []byte("somebody")
	var gotBody []byte
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(wantBody)),
		)
	})
	if !bytes.Equal(gotBody, wantBody) {
		t.Errorf("got body %q, want %q", string(gotBody), string(wantBody))
	}
}

func TestWithJSONRequestBody(t *testing.T) {
	type response struct {
		Message string `json:"message"`
	}
	message := "text"

	wantBody := response{
		Message: message,
	}
	var gotBody response
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, err := io.ReadAll(r.Body)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
			return
		}
		if err := json.Unmarshal(v, &gotBody); err != nil {
			jsonhttp.BadRequest(w, err)
			return
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithJSONRequestBody(wantBody),
		)
	})
	if gotBody.Message != message {
		t.Errorf("got message %q, want %q", gotBody.Message, message)
	}
}

func TestWithMultipartRequest(t *testing.T) {
	wantBody := []byte("somebody")
	filename := "swarm.jpg"
	contentType := "image/jpeg"
	var gotBody []byte
	var gotContentDisposition, gotContentType string
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			jsonhttp.BadRequest(w, err)
			return
		}
		if strings.HasPrefix(mediaType, "multipart/") {
			mr := multipart.NewReader(r.Body, params["boundary"])

			p, err := mr.NextPart()
			if err != nil {
				if err == io.EOF {
					return
				}
				jsonhttp.BadRequest(w, err)
				return
			}
			gotContentDisposition = p.Header.Get("Content-Disposition")
			gotContentType = p.Header.Get("Content-Type")
			gotBody, err = io.ReadAll(p)
			if err != nil {
				jsonhttp.BadRequest(w, err)
				return
			}
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithMultipartRequest(bytes.NewReader(wantBody), len(wantBody), filename, contentType),
		)
	})
	if !bytes.Equal(gotBody, wantBody) {
		t.Errorf("got body %q, want %q", string(gotBody), string(wantBody))
	}
	if gotContentType != contentType {
		t.Errorf("got content type %q, want %q", gotContentType, contentType)
	}
	if contentDisposition := fmt.Sprintf("form-data; name=%q", filename); gotContentDisposition != contentDisposition {
		t.Errorf("got content disposition %q, want %q", gotContentDisposition, contentDisposition)
	}
}

func TestWithRequestHeader(t *testing.T) {
	headerName := "Swarm-Header"
	headerValue := "somevalue"
	var gotValue string

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValue = r.Header.Get(headerName)
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithRequestHeader(headerName, headerValue),
		)
	})
	if gotValue != headerValue {
		t.Errorf("got header %q, want %q", gotValue, headerValue)
	}
}

func TestWithExpectedResponse(t *testing.T) {
	body := []byte("something to want")

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(body)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedResponse(body),
		)
	})

	assert(t, `got response "something to want", want "invalid"`, "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedResponse([]byte("invalid")),
		)
	})
}

func TestWithExpectedJSONResponse(t *testing.T) {
	type response struct {
		Message string `json:"message"`
	}

	want := response{
		Message: "text",
	}

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonhttp.OK(w, want)
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(want),
		)
	})

	assert(t, `got json response "{\"message\":\"text\"}", want "{\"message\":\"invalid\"}"`, "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(response{
				Message: "invalid",
			}),
		)
	})
}

func TestWithUnmarhalJSONResponse(t *testing.T) {
	message := "text"

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonhttp.OK(w, message)
	}))

	var r jsonhttp.StatusResponse
	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&r),
		)
	})
	if r.Message != message {
		t.Errorf("got message %q, want %q", r.Message, message)
	}
}

func TestWithPutResponseBody(t *testing.T) {
	wantBody := []byte("somebody")

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(wantBody)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
		}
	}))

	var gotBody []byte
	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithPutResponseBody(&gotBody),
		)
	})
	if !bytes.Equal(gotBody, wantBody) {
		t.Errorf("got body %q, want %q", string(gotBody), string(wantBody))
	}
}

func TestWithNoResponseBody(t *testing.T) {
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			fmt.Fprint(w, "not found")
		}
	}))

	assert(t, "", "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithNoResponseBody(),
		)
	})

	assert(t, `got response body "not found", want none`, "", func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint+"/bzz", http.StatusOK,
			jsonhttptest.WithNoResponseBody(),
		)
	})
}

func newClient(t *testing.T, handler http.Handler) (c *http.Client, endpoint string) {
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s.Client(), s.URL
}
