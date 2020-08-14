// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

func Request(t *testing.T, client *http.Client, method, url string, responseCode int, opts ...Option) http.Header {
	t.Helper()

	o := new(options)
	for _, opt := range opts {
		if err := opt.apply(o); err != nil {
			t.Fatal(err)
		}
	}

	req, err := http.NewRequest(method, url, o.requestBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = o.requestHeaders
	if o.ctx != nil {
		req = req.WithContext(o.ctx)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}

	if o.expectedResponse != nil {
		got, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, o.expectedResponse) {
			t.Errorf("got response %s, want %s", string(got), string(o.expectedResponse))
		}
		return resp.Header
	}

	if o.expectedJSONResponse != nil {
		if v := resp.Header.Get("Content-Type"); v != jsonhttp.DefaultContentTypeHeader {
			t.Errorf("got content type %q, want %q", v, jsonhttp.DefaultContentTypeHeader)
		}
		got, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		got = bytes.TrimSpace(got)

		want, err := json.Marshal(o.expectedJSONResponse)
		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("got json response %s, want %s", string(got), string(want))
		}
		return resp.Header
	}

	if o.unmarshalResponse != nil {
		if err := json.NewDecoder(resp.Body).Decode(&o.unmarshalResponse); err != nil {
			t.Fatal(err)
		}
		return resp.Header
	}
	if o.responseBody != nil {
		got, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		*o.responseBody = got
	}
	if o.noResponseBody {
		got, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > 0 {
			t.Errorf("got response body %s, want none", string(got))
		}
	}
	return resp.Header
}

func WithContext(ctx context.Context) Option {
	return optionFunc(func(o *options) error {
		o.ctx = ctx
		return nil
	})
}

func WithRequestBody(body io.Reader) Option {
	return optionFunc(func(o *options) error {
		o.requestBody = body
		return nil
	})
}

func WithJSONRequestBody(r interface{}) Option {
	return optionFunc(func(o *options) error {
		b, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("json encode request body: %w", err)
		}
		o.requestBody = bytes.NewReader(b)
		return nil
	})
}

func WithMultipartRequest(body io.Reader, length int, filename, contentType string) Option {
	return optionFunc(func(o *options) error {
		buf := bytes.NewBuffer(nil)
		mw := multipart.NewWriter(buf)
		hdr := make(textproto.MIMEHeader)
		if filename != "" {
			hdr.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q", filename))
		}
		if contentType != "" {
			hdr.Set("Content-Type", contentType)
		}
		if length > 0 {
			hdr.Set("Content-Length", strconv.Itoa(length))
		}
		part, err := mw.CreatePart(hdr)
		if err != nil {
			return err
		}
		if _, err = io.Copy(part, body); err != nil {
			return err
		}
		if err := mw.Close(); err != nil {
			return err
		}
		o.requestBody = buf
		if o.requestHeaders == nil {
			o.requestHeaders = make(http.Header)
		}
		o.requestHeaders.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%q", mw.Boundary()))
		return nil
	})
}

func WithRequestHeader(key, value string) Option {
	return optionFunc(func(o *options) error {
		if o.requestHeaders == nil {
			o.requestHeaders = make(http.Header)
		}
		o.requestHeaders.Add(key, value)
		return nil
	})
}

func WithExpectedResponse(response []byte) Option {
	return optionFunc(func(o *options) error {
		o.expectedResponse = response
		return nil
	})
}

func WithExpectedJSONResponse(response interface{}) Option {
	return optionFunc(func(o *options) error {
		o.expectedJSONResponse = response
		return nil
	})
}

func WithUnmarshalResponse(response interface{}) Option {
	return optionFunc(func(o *options) error {
		o.unmarshalResponse = response
		return nil
	})
}

func WithPutResponseBody(b *[]byte) Option {
	return optionFunc(func(o *options) error {
		o.responseBody = b
		return nil
	})
}

func WithNoResponseBody() Option {
	return optionFunc(func(o *options) error {
		o.noResponseBody = true
		return nil
	})
}

type options struct {
	ctx                  context.Context
	requestBody          io.Reader
	requestHeaders       http.Header
	expectedResponse     []byte
	expectedJSONResponse interface{}
	unmarshalResponse    interface{}
	responseBody         *[]byte
	noResponseBody       bool
}

type Option interface {
	apply(*options) error
}
type optionFunc func(*options) error

func (f optionFunc) apply(r *options) error { return f(r) }
