// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	requestTimeout = 10 * time.Second
	// maxHeaderBytes bounds the response headers of the registry.
	maxHeaderBytes = 16 << 10 // 16 KiB
	maxRedirects   = 5
	// maxETagSize bounds an entity tag that is remembered for a conditional
	// request.
	maxETagSize = 256
)

var (
	errNotOK               = errors.New("updatecheck: unexpected response status")
	errResponseTooLarge    = errors.New("updatecheck: response body too large")
	errCrossOriginRedirect = errors.New("updatecheck: redirect to another origin refused")
	errTooManyRedirects    = errors.New("updatecheck: too many redirects")
)

// newClient returns the default HTTP client for registry requests.
func newClient() *http.Client {
	return &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			ForceAttemptHTTP2:      true,
			TLSHandshakeTimeout:    requestTimeout,
			ResponseHeaderTimeout:  requestTimeout,
			IdleConnTimeout:        90 * time.Second,
			MaxIdleConns:           2,
			MaxResponseHeaderBytes: maxHeaderBytes,
		},
		CheckRedirect: sameOriginRedirect,
	}
}

// downloadHeaderTimeout bounds how long the registry may take to start
// answering a binary download. A gateway retrieves a cold ~75 MB file from
// Swarm before it sends headers, which takes far longer than requestTimeout.
const downloadHeaderTimeout = 2 * time.Minute

// newDownloadClient returns the client for binary downloads: client when one
// was configured, otherwise the default client without its overall request
// timeout and with a longer response header timeout, both of which a cold
// download through a gateway exceeds. The caller bounds the download with a
// context instead.
func newDownloadClient(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	c := newClient()
	c.Timeout = 0
	t := c.Transport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = downloadHeaderTimeout
	c.Transport = t
	return c
}

// sameOriginRedirect follows a redirect only within the registry's origin, so
// that a registry cannot point bee at an arbitrary host.
func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	if req.URL.Scheme != via[0].URL.Scheme || req.URL.Host != via[0].URL.Host {
		return errCrossOriginRedirect
	}
	return nil
}

// cachedResponse is the last successful response for a URL that came with an
// entity tag.
type cachedResponse struct {
	etag string
	body []byte
}

// httpGetter fetches registry documents, revalidating the last response for
// a URL with If-None-Match when the registry gave it an entity tag.
type httpGetter struct {
	client *http.Client

	mu    sync.Mutex
	cache map[string]cachedResponse
}

func newHTTPGetter(client *http.Client) *httpGetter {
	if client == nil {
		client = newClient()
	}
	return &httpGetter{client: client, cache: make(map[string]cachedResponse)}
}

// get fetches target and returns its body. A body of limit bytes or more is
// refused, as bee-runner refuses it. Neither the status line nor the body of
// an unexpected response is returned, since both are controlled by the
// registry and end up in logs.
func (g *httpGetter) get(ctx context.Context, target string, limit int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	g.mu.Lock()
	cached, isCached := g.cache[target]
	g.mu.Unlock()
	if isCached {
		req.Header.Set("If-None-Match", cached.etag)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		// Drop the URL: it may carry credentials, or be a redirect target
		// chosen by the registry.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return nil, fmt.Errorf("get: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, int64(limit)))
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusNotModified && isCached:
		return cached.body, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: %d %s", errNotOK, resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(body) >= limit {
		return nil, fmt.Errorf("%w: %d bytes or more", errResponseTooLarge, limit)
	}

	g.mu.Lock()
	if etag := resp.Header.Get("ETag"); etag != "" && len(etag) <= maxETagSize {
		g.cache[target] = cachedResponse{etag: etag, body: body}
	} else {
		delete(g.cache, target)
	}
	g.mu.Unlock()

	return body, nil
}
