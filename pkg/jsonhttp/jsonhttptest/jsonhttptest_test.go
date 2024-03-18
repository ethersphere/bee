// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
)

func TestRequest_statusCode(t *testing.T) {
	t.Parallel()

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusBadRequest)
	})

	tr := testResult{
		errors: []string{`got response status 400 Bad Request, want 200 OK`},
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})
}

func TestRequest_method(t *testing.T) {
	t.Parallel()

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusMethodNotAllowed)
	})
}

func TestRequest_url(t *testing.T) {
	t.Parallel()

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK)
	})

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint+"/bzz", http.StatusNotFound)
	})
}

func TestRequest_responseHeader(t *testing.T) {
	t.Parallel()

	headerName := "Swarm-Header"
	headerValue := "Digital Freedom Now"

	t.Run("with header", func(t *testing.T) {
		t.Parallel()

		c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(headerName, headerValue)
		}))

		assert(t, testResult{}, func(m *mock) {
			jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
				jsonhttptest.WithExpectedResponseHeader(headerName, headerValue),
				jsonhttptest.WithNonEmptyResponseHeader(headerName),
			)
		})
	})

	t.Run("without header", func(t *testing.T) {
		t.Parallel()

		c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		tr := testResult{
			errors: []string{
				fmt.Sprintf(`header key=[%v] should be set`, headerName),
				fmt.Sprintf(`header values for key=[%v] not as expected, got: [], want [%v]`, headerName, headerValue),
			},
		}

		assert(t, tr, func(m *mock) {
			jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
				jsonhttptest.WithNonEmptyResponseHeader(headerName),
				jsonhttptest.WithExpectedResponseHeader(headerName, headerValue),
			)
		})
	})
}

func TestWithContext(t *testing.T) {
	t.Parallel()

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel the context to detect the fatal error

	tr := testResult{
		fatal: fmt.Sprintf("Get %q: context canceled", endpoint),
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithContext(ctx),
		)
	})
}

func TestWithRequestBody(t *testing.T) {
	t.Parallel()

	wantBody := []byte("somebody")
	var gotBody []byte
	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithRequestBody(bytes.NewReader(wantBody)),
		)
	})
	if !bytes.Equal(gotBody, wantBody) {
		t.Errorf("got body %q, want %q", string(gotBody), string(wantBody))
	}
}

func TestWithJSONRequestBody(t *testing.T) {
	t.Parallel()

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

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithJSONRequestBody(wantBody),
		)
	})
	if gotBody.Message != message {
		t.Errorf("got message %q, want %q", gotBody.Message, message)
	}
}

func TestWithMultipartRequest(t *testing.T) {
	t.Parallel()

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
				if errors.Is(err, io.EOF) {
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

	assert(t, testResult{}, func(m *mock) {
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
	t.Parallel()

	headerName := "Swarm-Header"
	headerValue := "somevalue"
	var gotValue string

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotValue = r.Header.Get(headerName)
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodPost, endpoint, http.StatusOK,
			jsonhttptest.WithRequestHeader(headerName, headerValue),
		)
	})
	if gotValue != headerValue {
		t.Errorf("got header %q, want %q", gotValue, headerValue)
	}
}

func TestWithExpectedContentLength(t *testing.T) {
	t.Parallel()

	body := []byte("Digital Freedom Now")

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, err := w.Write(body)
		if err != nil {
			t.Error(err)
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedContentLength(len(body)),
		)
	})

	tr := testResult{
		errors: []string{
			"header values for key=[Content-Length] not as expected, got: [19], want [100]",
			"http.Response.ContentLength not as expected, got 19, want 100",
		},
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedContentLength(100),
		)
	})
}

func TestWithExpectedResponse(t *testing.T) {
	t.Parallel()

	body := []byte("something to want")

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(body)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedResponse(body),
		)
	})

	tr := testResult{
		errors: []string{`got response "something to want", want "invalid"`},
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedResponse([]byte("invalid")),
		)
	})
}

func TestWithExpectedJSONResponse(t *testing.T) {
	t.Parallel()

	type response struct {
		Message string `json:"message"`
	}

	want := response{
		Message: "text",
	}

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonhttp.OK(w, want)
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(want),
		)
	})

	tr := testResult{
		errors: []string{`got json response "{\"message\":\"text\"}", want "{\"message\":\"invalid\"}"`},
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(response{
				Message: "invalid",
			}),
		)
	})
}

func TestWithUnmarhalJSONResponse(t *testing.T) {
	t.Parallel()

	message := "text"

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonhttp.OK(w, message)
	}))

	var r jsonhttp.StatusResponse
	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&r),
		)
	})
	if r.Message != message {
		t.Errorf("got message %q, want %q", r.Message, message)
	}
}

func TestWithPutResponseBody(t *testing.T) {
	t.Parallel()

	wantBody := []byte("somebody")

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write(wantBody)
		if err != nil {
			jsonhttp.InternalServerError(w, err)
		}
	}))

	var gotBody []byte
	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithPutResponseBody(&gotBody),
		)
	})
	if !bytes.Equal(gotBody, wantBody) {
		t.Errorf("got body %q, want %q", string(gotBody), string(wantBody))
	}
}

func TestWithNoResponseBody(t *testing.T) {
	t.Parallel()

	c, endpoint := newClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			fmt.Fprint(w, "not found")
		}
	}))

	assert(t, testResult{}, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint, http.StatusOK,
			jsonhttptest.WithNoResponseBody(),
		)
	})

	tr := testResult{
		errors: []string{`got response body "not found", want none`},
	}
	assert(t, tr, func(m *mock) {
		jsonhttptest.Request(m, c, http.MethodGet, endpoint+"/bzz", http.StatusOK,
			jsonhttptest.WithNoResponseBody(),
		)
	})
}

func newClient(t *testing.T, handler http.Handler) (c *http.Client, endpoint string) {
	t.Helper()

	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s.Client(), s.URL
}
