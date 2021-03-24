// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf_test

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/protobuf/internal/pb"
)

func TestReader_ReadMsg(t *testing.T) {
	messages := []string{"first", "second", "third"}

	for _, tc := range []struct {
		name       string
		readerFunc func() protobuf.Reader
	}{
		{
			name: "NewReader",
			readerFunc: func() protobuf.Reader {
				return protobuf.NewReader(newMessageReader(messages, 0))
			},
		},
		{
			name: "NewWriterAndReader",
			readerFunc: func() protobuf.Reader {
				_, r := protobuf.NewWriterAndReader(
					newNoopWriteCloser(
						newMessageReader(messages, 0),
					),
				)
				return r
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.readerFunc()
			var msg pb.Message
			for i := 0; i < len(messages); i++ {
				err := r.ReadMsg(&msg)
				if i == len(messages) {
					if err != io.EOF {
						t.Fatalf("got error %v, want %v", err, io.EOF)
					}
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				want := messages[i]
				got := msg.Text
				if got != want {
					t.Errorf("got message %q, want %q", got, want)
				}
			}
		})
	}
}

func TestReader_timeout(t *testing.T) {
	messages := []string{"first", "second", "third"}

	for _, tc := range []struct {
		name       string
		readerFunc func() protobuf.Reader
	}{
		{
			name: "NewReader",
			readerFunc: func() protobuf.Reader {
				return protobuf.NewReader(newMessageReader(messages, 500*time.Millisecond))
			},
		},
		{
			name: "NewWriterAndReader",
			readerFunc: func() protobuf.Reader {
				_, r := protobuf.NewWriterAndReader(
					newNoopWriteCloser(
						newMessageReader(messages, 500*time.Millisecond),
					),
				)
				return r
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("WithContext", func(t *testing.T) {
				r := tc.readerFunc()
				var msg pb.Message
				for i := 0; i < len(messages); i++ {
					var timeout time.Duration
					if i == 0 {
						timeout = 1000 * time.Millisecond
					} else {
						timeout = 10 * time.Millisecond
					}
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					err := r.ReadMsgWithContext(ctx, &msg)
					if i == 0 {
						if err != nil {
							t.Fatal(err)
						}
					} else {
						if err != context.DeadlineExceeded {
							t.Fatalf("got error %v, want %v", err, context.DeadlineExceeded)
						}
						break
					}
					want := messages[i]
					got := msg.Text
					if got != want {
						t.Errorf("got message %q, want %q", got, want)
					}
				}
			})
		})
	}
}

func TestWriter(t *testing.T) {
	messages := []string{"first", "second", "third"}

	for _, tc := range []struct {
		name       string
		writerFunc func() (protobuf.Writer, <-chan string)
	}{
		{
			name: "NewWriter",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(0)
				return protobuf.NewWriter(w), msgs
			},
		},
		{
			name: "NewWriterAndReader",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(0)
				writer, _ := protobuf.NewWriterAndReader(newNoopReadCloser(w))
				return writer, msgs
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, msgs := tc.writerFunc()

			for _, m := range messages {
				if err := w.WriteMsg(&pb.Message{
					Text: m,
				}); err != nil {
					t.Fatal(err)
				}

				if got := <-msgs; got != m {
					t.Fatalf("got message %q, want %q", got, m)
				}
			}
		})
	}
}

func TestWriter_timeout(t *testing.T) {
	messages := []string{"first", "second", "third"}

	for _, tc := range []struct {
		name       string
		writerFunc func() (protobuf.Writer, <-chan string)
	}{
		{
			name: "NewWriter",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(500 * time.Millisecond)
				return protobuf.NewWriter(w), msgs
			},
		},
		{
			name: "NewWriterAndReader",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(500 * time.Millisecond)
				writer, _ := protobuf.NewWriterAndReader(newNoopReadCloser(w))
				return writer, msgs
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("WithContext", func(t *testing.T) {
				w, msgs := tc.writerFunc()

				for i, m := range messages {
					var timeout time.Duration
					if i == 0 {
						timeout = 1000 * time.Millisecond
					} else {
						timeout = 10 * time.Millisecond
					}
					ctx, cancel := context.WithTimeout(context.Background(), timeout)
					defer cancel()
					err := w.WriteMsgWithContext(ctx, &pb.Message{
						Text: m,
					})
					if i == 0 {
						if err != nil {
							t.Fatal(err)
						}
					} else {
						if err != context.DeadlineExceeded {
							t.Fatalf("got error %v, want %v", err, context.DeadlineExceeded)
						}
						break
					}
					if got := <-msgs; got != m {
						t.Fatalf("got message %q, want %q", got, m)
					}
				}
			})
		})
	}
}

func TestReadMessages(t *testing.T) {
	messages := []string{"first", "second", "third"}

	r := newMessageReader(messages, 0)

	got, err := protobuf.ReadMessages(r, func() protobuf.Message { return new(pb.Message) })
	if err != nil {
		t.Fatal(err)
	}

	var gotMessages []string
	for _, m := range got {
		gotMessages = append(gotMessages, m.(*pb.Message).Text)
	}

	if fmt.Sprint(gotMessages) != fmt.Sprint(messages) {
		t.Errorf("got messages %v, want %v", gotMessages, messages)
	}
}

func newMessageReader(messages []string, delay time.Duration) io.Reader {
	r, pipe := io.Pipe()
	w := protobuf.NewWriter(pipe)

	go func() {
		for _, m := range messages {
			if err := w.WriteMsg(&pb.Message{
				Text: m,
			}); err != nil {
				panic(err)
			}
		}
		if err := pipe.Close(); err != nil {
			panic(err)
		}
	}()

	return delayedReader{r: r, delay: delay}
}

func newMessageWriter(delay time.Duration) (w io.Writer, messages <-chan string) {
	pipe, w := io.Pipe()
	r := protobuf.NewReader(pipe)
	msgs := make(chan string)

	go func() {
		defer close(msgs)

		var msg pb.Message
		for {
			err := r.ReadMsg(&msg)
			if err != nil {
				if err == io.EOF {
					return
				}
				panic(err)
			}
			msgs <- msg.Text
		}
	}()
	return delayedWriter{w: w, delay: delay}, msgs
}

type delayedWriter struct {
	w     io.Writer
	delay time.Duration
}

func (d delayedWriter) Write(p []byte) (n int, err error) {
	time.Sleep(d.delay)
	return d.w.Write(p)
}

type delayedReader struct {
	r     io.Reader
	delay time.Duration
}

func (d delayedReader) Read(p []byte) (n int, err error) {
	time.Sleep(d.delay)
	return d.r.Read(p)
}

type noopWriteCloser struct {
	io.Reader
}

func newNoopWriteCloser(r io.Reader) noopWriteCloser {
	return noopWriteCloser{Reader: r}
}

func (noopWriteCloser) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (noopWriteCloser) Headers() p2p.Headers {
	return nil
}

func (noopWriteCloser) ResponseHeaders() p2p.Headers {
	return nil
}

func (noopWriteCloser) Close() error {
	return nil
}

func (noopWriteCloser) FullClose() error {
	return nil
}

func (noopWriteCloser) Reset() error {
	return nil
}

type noopReadCloser struct {
	io.Writer
}

func newNoopReadCloser(w io.Writer) noopReadCloser {
	return noopReadCloser{Writer: w}
}

func (noopReadCloser) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (noopReadCloser) Headers() p2p.Headers {
	return nil
}

func (noopReadCloser) ResponseHeaders() p2p.Headers {
	return nil
}

func (noopReadCloser) Close() error {
	return nil
}

func (noopReadCloser) FullClose() error {
	return nil
}

func (noopReadCloser) Reset() error {
	return nil
}
