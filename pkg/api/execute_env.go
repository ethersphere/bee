// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/compute"
)

// The request metadata a module sees, CGI-style. Only the method used to be
// exposed, which left a module with one URL and no way to tell one request from
// another. The names and their meanings are CGI's, because that is the
// convention REQUEST_METHOD already committed the ABI to.
const (
	envScriptName    = "SCRIPT_NAME"
	envPathInfo      = "PATH_INFO"
	envQueryString   = "QUERY_STRING"
	envRequestURI    = "REQUEST_URI"
	envContentType   = "CONTENT_TYPE"
	envContentLength = "CONTENT_LENGTH"
	// envHeaderPrefix is CGI's prefix for a request header: Accept-Language
	// becomes HTTP_ACCEPT_LANGUAGE.
	envHeaderPrefix = "HTTP_"
)

// defaultRequestHeaders are the request headers a module may see when the
// operator has not configured a list.
//
// This is an allowlist, deliberately unlike the response direction's denylist. A
// response header carries only what the guest already knows; a request header
// carries what the *operator's* clients send, so nothing gets through that is
// not named here.
//
// Absent on purpose: Authorization, Cookie and Proxy-Authorization are node
// credentials; Origin is the node's CORS business and would let a module
// fingerprint the pages embedding it; Accept-Encoding belongs to the node, which
// owns transfer encoding; X-Forwarded-For and Forwarded carry a visitor's IP,
// which a module could persist to Swarm permanently.
var defaultRequestHeaders = []string{
	"Accept",
	"Accept-Language",
	"Host",
	"If-None-Match",
	"If-Modified-Since",
	"Range",
	"Referer",
	"User-Agent",
	"X-Requested-With",
	// Client-supplied and already accepted by the CORS layer. A module cannot
	// enumerate the node's batches, so this is how a caller hands it one to
	// upload with.
	"Swarm-Postage-Batch-Id",
}

// forbiddenRequestHeaders may never be forwarded, whatever an operator
// configures. Authorization together with the Access-Control-Allow-Credentials
// the node sets would mean handing an untrusted module the operator's token; an
// operator with a real need puts a proxy in front and passes a derived header.
var forbiddenRequestHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
}

// maxRequestHeaderValueLen is the longest header value forwarded to a module.
// A longer one is dropped rather than truncated: a truncated value is a lie the
// module cannot detect.
const maxRequestHeaderValueLen = 4 << 10

// ValidateRequestHeaders reports whether a configured request-header allowlist is
// safe to serve. It is checked at startup so a dangerous configuration stops the
// node rather than producing a warning nobody reads.
func ValidateRequestHeaders(names []string) error {
	for _, name := range names {
		if _, forbidden := forbiddenRequestHeaders[strings.ToLower(strings.TrimSpace(name))]; forbidden {
			return fmt.Errorf("request header %q may not be exposed to WASM modules", name)
		}
	}
	return nil
}

// cgiHeaderName mangles a header name the way CGI does: upper case, dashes to
// underscores, prefixed with HTTP_.
func cgiHeaderName(name string) string {
	return envHeaderPrefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// executeEnv derives the CGI environment for one request.
//
// pathInfo is passed in rather than recomputed because only the router knows
// whether the trailing-path route matched: on the bare route PATH_INFO is empty,
// which is how a module distinguishes /@/{addr} from /@/{addr}/.
func (s *Service) executeEnv(r *http.Request, pathInfo string) []compute.EnvVar {
	env := []compute.EnvVar{
		// SCRIPT_NAME is CGI's definition — the mount point, the request path
		// minus PATH_INFO. Deriving it this way rather than rebuilding it from
		// the address handles the /v1/@/... alias for free, and it is what a
		// module needs to build links and redirects to itself.
		{Name: envScriptName, Value: strings.TrimSuffix(r.URL.Path, pathInfo)},
		{Name: envPathInfo, Value: pathInfo},
		// Undecoded: a module that wants the decoded form decodes it, and one
		// that needs the raw bytes still has them.
		{Name: envQueryString, Value: r.URL.RawQuery},
		{Name: envRequestURI, Value: r.URL.RequestURI()},
	}

	// Per CGI these two describe the body and are not repeated as HTTP_*.
	if ct := r.Header.Get(ContentTypeHeader); ct != "" {
		env = append(env, compute.EnvVar{Name: envContentType, Value: ct})
	}
	if r.ContentLength >= 0 {
		env = append(env, compute.EnvVar{
			Name:  envContentLength,
			Value: strconv.FormatInt(r.ContentLength, 10),
		})
	}

	allowed := s.executeConfig.RequestHeaders
	if len(allowed) == 0 {
		allowed = defaultRequestHeaders
	}

	headers := make([]compute.EnvVar, 0, len(allowed))
	for _, name := range allowed {
		if _, forbidden := forbiddenRequestHeaders[strings.ToLower(name)]; forbidden {
			continue
		}
		values := r.Header.Values(name)
		if len(values) == 0 {
			continue
		}
		// Repeats join the way a single field-value would have been written.
		value := strings.Join(values, ", ")
		if len(value) > maxRequestHeaderValueLen {
			continue
		}
		headers = append(headers, compute.EnvVar{Name: cgiHeaderName(name), Value: value})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })

	return append(env, sanitiseEnv(headers)...)
}

// sanitiseEnv drops entries whose value carries a control character. Such a byte
// has no meaning in an environment block and a NUL would truncate it outright.
func sanitiseEnv(env []compute.EnvVar) []compute.EnvVar {
	out := env[:0]
	for _, v := range env {
		if strings.ContainsFunc(v.Value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			continue
		}
		out = append(out, v)
	}
	return out
}

// envSize is the number of bytes an environment block occupies, counting the
// "name=value\x00" framing WASI uses.
func envSize(env []compute.EnvVar) int {
	total := 0
	for _, v := range env {
		total += len(v.Name) + len(v.Value) + 2
	}
	return total
}
