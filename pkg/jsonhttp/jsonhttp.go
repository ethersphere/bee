// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Most of the code is copied from package resenje.org/jsonresponse.

package jsonhttp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

var (
	// DefaultContentTypeHeader is the value of if "Content-Type" header
	// in HTTP response.
	DefaultContentTypeHeader = "application/json; charset=utf-8"
	// EscapeHTML specifies whether problematic HTML characters
	// should be escaped inside JSON quoted strings.
	EscapeHTML = false
)

// StatusResponse is a standardized error format for specific HTTP responses.
// Code field corresponds with HTTP status code, and Message field is a short
// description of that code or provides more context about the reason for such
// response.
//
// If response is string, error or Stringer type the string will be set as
// value to the Message field.
type StatusResponse struct {
	Message string `json:"message,omitempty"`
	Code    int    `json:"code,omitempty"`
}

// Respond writes a JSON-encoded body to http.ResponseWriter.
func Respond(w http.ResponseWriter, statusCode int, response interface{}) {
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	if response == nil {
		response = &StatusResponse{
			Message: http.StatusText(statusCode),
			Code:    statusCode,
		}
	} else {
		switch message := response.(type) {
		case string:
			response = &StatusResponse{
				Message: message,
				Code:    statusCode,
			}
		case error:
			response = &StatusResponse{
				Message: message.Error(),
				Code:    statusCode,
			}
		case interface {
			String() string
		}:
			response = &StatusResponse{
				Message: message.String(),
				Code:    statusCode,
			}
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(EscapeHTML)
	if err := enc.Encode(response); err != nil {
		panic(err)
	}
	if DefaultContentTypeHeader != "" {
		w.Header().Set("Content-Type", DefaultContentTypeHeader)
	}
	w.WriteHeader(statusCode)
	fmt.Println("writing", b.String())
	fmt.Fprintln(w, b.String())
}

// Continue writes a response with status code 100.
func Continue(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusContinue, response)
}

// SwitchingProtocols writes a response with status code 101.
func SwitchingProtocols(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusSwitchingProtocols, response)
}

// OK writes a response with status code 200.
func OK(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusOK, response)
}

// Created writes a response with status code 201.
func Created(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusCreated, response)
}

// Accepted writes a response with status code 202.
func Accepted(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusAccepted, response)
}

// NonAuthoritativeInfo writes a response with status code 203.
func NonAuthoritativeInfo(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNonAuthoritativeInfo, response)
}

// NoContent writes a response with status code 204.
func NoContent(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNoContent, response)
}

// ResetContent writes a response with status code 205.
func ResetContent(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusResetContent, response)
}

// PartialContent writes a response with status code 206.
func PartialContent(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusPartialContent, response)
}

// MultipleChoices writes a response with status code 300.
func MultipleChoices(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusMultipleChoices, response)
}

// MovedPermanently writes a response with status code 301.
func MovedPermanently(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusMovedPermanently, response)
}

// Found writes a response with status code 302.
func Found(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusFound, response)
}

// SeeOther writes a response with status code 303.
func SeeOther(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusSeeOther, response)
}

// NotModified writes a response with status code 304.
func NotModified(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNotModified, response)
}

// UseProxy writes a response with status code 305.
func UseProxy(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusUseProxy, response)
}

// TemporaryRedirect writes a response with status code 307.
func TemporaryRedirect(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusTemporaryRedirect, response)
}

// PermanentRedirect writes a response with status code 308.
func PermanentRedirect(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusPermanentRedirect, response)
}

// BadRequest writes a response with status code 400.
func BadRequest(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusBadRequest, response)
}

// Unauthorized writes a response with status code 401.
func Unauthorized(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusUnauthorized, response)
}

// PaymentRequired writes a response with status code 402.
func PaymentRequired(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusPaymentRequired, response)
}

// Forbidden writes a response with status code 403.
func Forbidden(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusForbidden, response)
}

// NotFound writes a response with status code 404.
func NotFound(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNotFound, response)
}

// MethodNotAllowed writes a response with status code 405.
func MethodNotAllowed(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusMethodNotAllowed, response)
}

// NotAcceptable writes a response with status code 406.
func NotAcceptable(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNotAcceptable, response)
}

// ProxyAuthRequired writes a response with status code 407.
func ProxyAuthRequired(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusProxyAuthRequired, response)
}

// RequestTimeout writes a response with status code 408.
func RequestTimeout(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusRequestTimeout, response)
}

// Conflict writes a response with status code 409.
func Conflict(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusConflict, response)
}

// Gone writes a response with status code 410.
func Gone(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusGone, response)
}

// LengthRequired writes a response with status code 411.
func LengthRequired(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusLengthRequired, response)
}

// PreconditionFailed writes a response with status code 412.
func PreconditionFailed(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusPreconditionFailed, response)
}

// RequestEntityTooLarge writes a response with status code 413.
func RequestEntityTooLarge(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusRequestEntityTooLarge, response)
}

// RequestURITooLong writes a response with status code 414.
func RequestURITooLong(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusRequestURITooLong, response)
}

// UnsupportedMediaType writes a response with status code 415.
func UnsupportedMediaType(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusUnsupportedMediaType, response)
}

// RequestedRangeNotSatisfiable writes a response with status code 416.
func RequestedRangeNotSatisfiable(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusRequestedRangeNotSatisfiable, response)
}

// ExpectationFailed writes a response with status code 417.
func ExpectationFailed(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusExpectationFailed, response)
}

// Teapot writes a response with status code 418.
func Teapot(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusTeapot, response)
}

// UpgradeRequired writes a response with status code 426.
func UpgradeRequired(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusUpgradeRequired, response)
}

// PreconditionRequired writes a response with status code 428.
func PreconditionRequired(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusPreconditionRequired, response)
}

// TooManyRequests writes a response with status code 429.
func TooManyRequests(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusTooManyRequests, response)
}

// RequestHeaderFieldsTooLarge writes a response with status code 431.
func RequestHeaderFieldsTooLarge(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusRequestHeaderFieldsTooLarge, response)
}

// UnavailableForLegalReasons writes a response with status code 451.
func UnavailableForLegalReasons(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusUnavailableForLegalReasons, response)
}

// InternalServerError writes a response with status code 500.
func InternalServerError(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusInternalServerError, response)
}

// NotImplemented writes a response with status code 501.
func NotImplemented(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusNotImplemented, response)
}

// BadGateway writes a response with status code 502.
func BadGateway(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusBadGateway, response)
}

// ServiceUnavailable writes a response with status code 503.
func ServiceUnavailable(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusServiceUnavailable, response)
}

// GatewayTimeout writes a response with status code 504.
func GatewayTimeout(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusGatewayTimeout, response)
}

// HTTPVersionNotSupported writes a response with status code 505.
func HTTPVersionNotSupported(w http.ResponseWriter, response interface{}) {
	Respond(w, http.StatusHTTPVersionNotSupported, response)
}
