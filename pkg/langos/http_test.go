// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package langos_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/langos"
)

// TestHTTPResponse validates that the langos returns correct data
// over http test server and ServeContent function.
func TestHTTPResponse(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "test", time.Now(), langos.NewBufferedLangos(bytes.NewReader(data), bufferSize))
		}))
		defer ts.Close()

		res, err := http.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		got, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, data) {
			t.Fatalf("got invalid data (lengths: got %v, want %v)", len(got), len(data))
		}
	})
}

// TestHTTPResponse validates that the langos returns correct data
// over http test server and ServeContent function for http range requests.
func TestHTTPRangeResponse(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "test", time.Now(), langos.NewBufferedLangos(bytes.NewReader(data), bufferSize))
		}))
		defer ts.Close()

		for i := 0; i < 12; i++ {
			start := rand.Intn(dataSize)
			var end int
			if dataSize-1-start <= 0 {
				end = dataSize - 1
			} else {
				end = rand.Intn(dataSize-1-start) + start
			}
			rangeHeader := fmt.Sprintf("bytes=%v-%v", start, end)
			if i == 0 {
				// test open ended range
				end = dataSize - 1
				rangeHeader = fmt.Sprintf("bytes=%v-", start)
			}

			gotRangs := httpRangeRequest(t, ts.URL, rangeHeader)
			got := gotRangs[0]
			want := data[start : end+1]
			if !bytes.Equal(got, want) {
				t.Fatalf("got invalid data for range %s (lengths: got %v, want %v)", rangeHeader, len(got), len(want))
			}
		}
	})
}

// TestHTTPMultipleRangeResponse validates that the langos returns correct data
// over http test server and ServeContent function for http requests with multiple ranges.
func TestHTTPMultipleRangeResponse(t *testing.T) {
	multiSizeTester(t, func(t *testing.T, dataSize, bufferSize int) {
		data := randomData(t, dataSize)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "test", time.Now(), langos.NewBufferedLangos(bytes.NewReader(data), bufferSize))
		}))
		defer ts.Close()

		for i := 0; i < 12; i++ {
			var ranges [][2]int

			var wantParts [][2]int
			for i := rand.Intn(5); i >= 0; i-- {
				var beginning int
				if l := len(ranges); l > 0 {
					beginning = ranges[l-1][1] + 1
				}
				if beginning >= dataSize {
					break
				}
				start := rand.Intn(dataSize-beginning) + beginning
				var end int
				if dataSize-1-start <= 0 {
					end = dataSize - 1
				} else {
					end = rand.Intn(dataSize-1-start) + start
				}
				if l := len(wantParts); l > 0 && wantParts[l-1][0] == start && wantParts[l-1][1] == end {
					continue
				}
				ranges = append(ranges, [2]int{start, end})
				wantParts = append(wantParts, [2]int{start, end})
			}

			rangeHeader := "bytes="
			for i, r := range ranges {
				if i > 0 {
					rangeHeader += ", "
				}
				rangeHeader += fmt.Sprintf("%v-%v", r[0], r[1])
			}

			gotParts := httpRangeRequest(t, ts.URL, rangeHeader)

			if len(gotParts) != len(wantParts) {
				t.Fatalf("got %v parts for range %q, want %v", len(gotParts), rangeHeader, len(wantParts))
			}

			for i, w := range wantParts {
				got := gotParts[i]
				want := data[w[0] : w[1]+1]
				if !bytes.Equal(got, want) {
					t.Fatalf("got invalid data for range #%v %s (lengths: got %v, want %v)", i+1, rangeHeader, len(got), len(want))
				}
			}
		}
	})
}

func parseDataSize(t *testing.T, v string) (s int) {
	t.Helper()

	multiplier := 1
	for suffix, value := range map[string]int{
		"k": 1024,
		"M": 1024 * 1024,
	} {
		if strings.HasSuffix(v, suffix) {
			v = strings.TrimSuffix(v, suffix)
			multiplier = value
			break
		}
	}
	s, err := strconv.Atoi(v)
	if err != nil {
		t.Fatal(err)
	}
	return s * multiplier
}

func httpRangeRequest(t *testing.T, url, rangeHeader string) (parts [][]byte) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Add("Range", rangeHeader)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	mimetype, params, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if mimetype == "multipart/byteranges" {
		mr := multipart.NewReader(res.Body, params["boundary"])
		for part, err := mr.NextPart(); err == nil; part, err = mr.NextPart() {
			value, err := ioutil.ReadAll(part)
			if err != nil {
				t.Fatal(err)
			}
			parts = append(parts, value)
		}
	} else {
		value, err := ioutil.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, value)
	}

	return parts
}

// BenchmarkHTTPDelayedReaders measures time needed by test http server to serve the body
// using different readers.
//
// goos: darwin
// goarch: amd64
// pkg: github.com/ethersphere/swarm/api/http/langos
// BenchmarkHTTPDelayedReaders/static_direct-8			         	       8	 128278515 ns/op	 8389878 B/op	      24 allocs/op
// BenchmarkHTTPDelayedReaders/static_buffered-8			       	      43	  27465687 ns/op	 8389144 B/op	      22 allocs/op
// BenchmarkHTTPDelayedReaders/static_langos-8			         	     441	   2578510 ns/op	10264076 B/op	      63 allocs/op
// BenchmarkHTTPDelayedReaders/static_buffered_langos-8         	     493	   2591692 ns/op	10120822 B/op	      57 allocs/op
// BenchmarkHTTPDelayedReaders/random_direct-8                  	       3	 351496566 ns/op	 8389416 B/op	      23 allocs/op
// BenchmarkHTTPDelayedReaders/random_buffered-8                	      14	  90407289 ns/op	 8389294 B/op	      22 allocs/op
// BenchmarkHTTPDelayedReaders/random_langos-8                  	     430	   2771827 ns/op	10256494 B/op	      62 allocs/op
// BenchmarkHTTPDelayedReaders/random_buffered_langos-8         	     420	   2817784 ns/op	10115937 B/op	      57 allocs/op
func BenchmarkHTTPDelayedReaders(b *testing.B) {
	dataSize := 2 * 1024 * 1024
	bufferSize := 4 * 32 * 1024

	data := randomData(b, dataSize)

	for _, bc := range []struct {
		name      string
		newReader func() langos.Reader
	}{
		{
			name: "static direct",
			newReader: func() langos.Reader {
				return newDelayedReaderStatic(bytes.NewReader(data), defaultStaticDelays)
			},
		},
		{
			name: "static buffered",
			newReader: func() langos.Reader {
				return langos.NewBufferedReadSeeker(newDelayedReaderStatic(bytes.NewReader(data), defaultStaticDelays), bufferSize)
			},
		},
		{
			name: "static langos",
			newReader: func() langos.Reader {
				return langos.NewLangos(newDelayedReaderStatic(bytes.NewReader(data), defaultStaticDelays), bufferSize)
			},
		},
		{
			name: "static buffered langos",
			newReader: func() langos.Reader {
				return langos.NewBufferedLangos(newDelayedReaderStatic(bytes.NewReader(data), defaultStaticDelays), bufferSize)
			},
		},
		{
			name: "random direct",
			newReader: func() langos.Reader {
				return newDelayedReader(bytes.NewReader(data), randomDelaysFunc)
			},
		},
		{
			name: "random buffered",
			newReader: func() langos.Reader {
				return langos.NewBufferedReadSeeker(newDelayedReader(bytes.NewReader(data), randomDelaysFunc), bufferSize)
			},
		},
		{
			name: "random langos",
			newReader: func() langos.Reader {
				return langos.NewLangos(newDelayedReader(bytes.NewReader(data), randomDelaysFunc), bufferSize)
			},
		},
		{
			name: "random buffered langos",
			newReader: func() langos.Reader {
				return langos.NewBufferedLangos(newDelayedReader(bytes.NewReader(data), randomDelaysFunc), bufferSize)
			},
		},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.StopTimer()

			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, "test", time.Now(), bc.newReader())
			}))
			defer ts.Close()

			for i := 0; i < b.N; i++ {
				res, err := http.Get(ts.URL)
				if err != nil {
					b.Fatal(err)
				}

				b.StartTimer()
				got, err := ioutil.ReadAll(res.Body)
				b.StopTimer()

				res.Body.Close()
				if err != nil {
					b.Fatal(err)
				}
				if !bytes.Equal(got, data) {
					b.Fatalf("%v got invalid data (lengths: got %v, want %v)", i, len(got), len(data))
				}
			}
		})
	}
}
