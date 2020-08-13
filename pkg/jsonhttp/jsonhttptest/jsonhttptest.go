// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jsonhttptest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"testing"
)

func ResponseDirect(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, nil)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("got", string(got))
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}
}

func ResponseDirectWithMultiPart(t *testing.T, client *http.Client, method, url, fileName string, data []byte,
	responseCode int, contentType string, response interface{}) http.Header {
	body := bytes.NewBuffer(nil)
	mw := multipart.NewWriter(body)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf("form-data; name=%q", fileName))
	hdr.Set("Content-Type", contentType)
	hdr.Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Error(err)
	}
	_, err = io.Copy(part, bytes.NewReader(data))
	if err != nil {
		t.Error(err)
	}
	err = mw.Close()
	if err != nil {
		t.Error(err)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Error(err)
	}
	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%q", mw.Boundary()))

	res, err := client.Do(req)
	if err != nil {
		t.Error(err)
	}
	defer res.Body.Close()

	if res.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", res.Status, responseCode, http.StatusText(responseCode))
	}

	got, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}

	return res.Header
}
func ResponseDirectCheckBinaryResponse(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int,
	response []byte, headers http.Header) http.Header {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, headers)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	if !bytes.Equal(got, response) {
		t.Errorf("got response %s, want %s", string(got), string(response))
	}
	return resp.Header
}

func ResponseDirectSendHeadersAndReceiveHeaders(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int,
	response interface{}, headers http.Header) http.Header {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, headers)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	want, err := json.Marshal(response)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("got response %s, want %s", string(got), string(want))
	}

	return resp.Header
}

// ResponseDirectSendHeadersAndDontCheckResponse sends a request with the given headers and does not check for the returned reference.
// this is useful in tests which does not know the return reference, for ex: when encryption flag is set
func ResponseDirectSendHeadersAndDontCheckResponse(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, headers http.Header) (http.Header, []byte) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, headers)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)

	return resp.Header, got
}

func ResponseUnmarshal(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, response interface{}) {
	t.Helper()

	resp := request(t, client, method, url, body, responseCode, nil)
	defer resp.Body.Close()

	got, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	got = bytes.TrimSpace(got)
	fmt.Println("got", string(got))

	if err := json.Unmarshal(got, &response); err != nil {
		t.Fatal(err)
	}
}

func request(t *testing.T, client *http.Client, method, url string, body io.Reader, responseCode int, headers http.Header) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header = headers
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != responseCode {
		t.Errorf("got response status %s, want %v %s", resp.Status, responseCode, http.StatusText(responseCode))
	}
	return resp
}
