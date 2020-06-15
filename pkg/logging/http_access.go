// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package logging

import (
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// NewHTTPAccessLogHandler creates a handler that will log a message after a
// request has been served.
func NewHTTPAccessLogHandler(logger Logger, level logrus.Level, message string) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startTime := time.Now()
			rl := &responseLogger{w, 0, 0, level}

			h.ServeHTTP(rl, r)

			if rl.level == 0 {
				return
			}

			status := rl.status
			if status == 0 {
				status = http.StatusOK
			}
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			fields := logrus.Fields{
				"ip":       ip,
				"method":   r.Method,
				"uri":      r.RequestURI,
				"proto":    r.Proto,
				"status":   status,
				"size":     rl.size,
				"duration": time.Since(startTime).Seconds(),
			}
			if v := r.Referer(); v != "" {
				fields["referrer"] = v
			}
			if v := r.UserAgent(); v != "" {
				fields["user-agent"] = v
			}
			if v := r.Header.Get("X-Forwarded-For"); v != "" {
				fields["x-forwarded-for"] = v
			}
			if v := r.Header.Get("X-Real-Ip"); v != "" {
				fields["x-real-ip"] = v
			}
			logger.WithFields(fields).Log(rl.level, message)
		})
	}
}

// SetAccessLogLevelHandler overrides the log level set in
// NewHTTPAccessLogHandler for a specific endpoint. Use log level 0 to suppress
// log messages.
func SetAccessLogLevelHandler(level logrus.Level) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if rl, ok := w.(*responseLogger); ok {
				rl.level = level
			}
			h.ServeHTTP(w, r)
		})
	}
}

type responseLogger struct {
	w      http.ResponseWriter
	status int
	size   int
	level  logrus.Level
}

func (l *responseLogger) Header() http.Header {
	return l.w.Header()
}

func (l *responseLogger) Flush() {
	l.w.(http.Flusher).Flush()
}

func (l *responseLogger) CloseNotify() <-chan bool {
	// staticcheck SA1019 CloseNotifier interface is required by gorilla compress handler
	// nolint:staticcheck
	return l.w.(http.CloseNotifier).CloseNotify() // skipcq: SCC-SA1019
}

func (l *responseLogger) Push(target string, opts *http.PushOptions) error {
	return l.w.(http.Pusher).Push(target, opts)
}

func (l *responseLogger) Write(b []byte) (int, error) {
	size, err := l.w.Write(b)
	l.size += size
	return size, err
}

func (l *responseLogger) WriteHeader(s int) {
	l.w.WriteHeader(s)
	if l.status == 0 {
		l.status = s
	}
}
