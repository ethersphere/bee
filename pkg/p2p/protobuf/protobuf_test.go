// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package protobuf_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/pkg/p2p/protobuf/internal/pb"
)

func TestReader_ReadMsg(t *testing.T) {
	t.Parallel()

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
				_, r := protobuf.NewWriterAndReader(newNoopWriteCloser(newMessageReader(messages, 0)))
				return r
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := tc.readerFunc()
			defer r.Close()

			var msg pb.Message
			for i := 0; i < len(messages); i++ {
				err := r.ReadMsg(&msg)
				if i == len(messages) {
					if !errors.Is(err, io.EOF) {
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
	t.Parallel()

	const delay = 500 * time.Millisecond

	messages := []string{"first", "second", "third"}

	for _, tc := range []struct {
		name       string
		readerFunc func() protobuf.Reader
	}{
		{
			name: "NewReader",
			readerFunc: func() protobuf.Reader {
				return protobuf.NewReader(newMessageReader(messages, delay))
			},
		},
		{
			name: "NewWriterAndReader",
			readerFunc: func() protobuf.Reader {
				_, r := protobuf.NewWriterAndReader(newNoopWriteCloser(newMessageReader(messages, delay)))
				return r
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := tc.readerFunc()
			defer r.Close()

			for i := 0; i < len(messages); i++ {
				var want string
				var msg pb.Message
				var timeout time.Duration
				if i == 0 {
					timeout = delay * 2
				} else {
					timeout = delay / 50
				}
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				err := r.ReadMsgWithContext(ctx, &msg)
				if i == 0 {
					if err != nil {
						t.Fatal(err)
					}
					want = messages[i]
				} else {
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("got error %v, want %v", err, context.DeadlineExceeded)
					}
					want = ""
				}

				got := msg.Text
				if got != want {
					t.Errorf("got message %q, want %q", got, want)
				}
			}
		})
	}
}

func TestWriter(t *testing.T) {
	t.Parallel()

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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w, msgs := tc.writerFunc()
			defer w.Close()

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

	const delay = 500 * time.Millisecond

	for _, tc := range []struct {
		name       string
		writerFunc func() (protobuf.Writer, <-chan string)
	}{
		{
			name: "NewWriter",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(delay)
				return protobuf.NewWriter(w), msgs
			},
		},
		{
			name: "NewWriterAndReader",
			writerFunc: func() (protobuf.Writer, <-chan string) {
				w, msgs := newMessageWriter(delay)
				writer, _ := protobuf.NewWriterAndReader(newNoopReadCloser(w))
				return writer, msgs
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w, msgs := tc.writerFunc()
			defer w.Close()

			for i, m := range messages {
				var timeout time.Duration
				if i == 0 {
					timeout = delay * 2
				} else {
					timeout = delay / 50
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
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("got error %v, want %v", err, context.DeadlineExceeded)
					}
					continue
				}
			}

			if got := <-msgs; got != messages[0] {
				t.Fatalf("got message %q, want %q", got, messages[0])
			}
		})
	}
}

func TestReadMessages(t *testing.T) {
	t.Parallel()

	messages := []string{"first", "second", "third"}

	r := newMessageReader(messages, 0)

	got, err := protobuf.ReadMessages(r, func() protobuf.Message { return new(pb.Message) })
	if err != nil {
		t.Fatal(err)
	}

	gotMessages := make([]string, 0, len(got))
	for _, m := range got {
		gotMessages = append(gotMessages, m.(*pb.Message).Text)
	}

	if fmt.Sprint(gotMessages) != fmt.Sprint(messages) {
		t.Errorf("got messages %v, want %v", gotMessages, messages)
	}
}

func newMessageReader(messages []string, delay time.Duration) io.ReadCloser {
	r, pipe := io.Pipe()
	w := protobuf.NewWriter(pipe)

	go func() {
		for _, m := range messages {
			if err := w.WriteMsg(&pb.Message{
				Text: m,
			}); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				panic(err)
			}
		}
		if err := w.Close(); err != nil {
			panic(err)
		}
	}()

	return delayedReader{r: r, delay: delay}
}

func newMessageWriter(delay time.Duration) (w io.WriteCloser, messages <-chan string) {
	pipe, w := io.Pipe()
	r := protobuf.NewReader(pipe)
	msgs := make(chan string, 16)

	go func() {
		defer close(msgs)

		for {
			var msg pb.Message
			err := r.ReadMsg(&msg)
			if err != nil {
				if errors.Is(err, io.EOF) {
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
	w     io.WriteCloser
	delay time.Duration
}

func (d delayedWriter) Write(p []byte) (n int, err error) {
	time.Sleep(d.delay)
	return d.w.Write(p)
}

func (d delayedWriter) Close() error {
	return d.w.Close()
}

type delayedReader struct {
	r     io.ReadCloser
	delay time.Duration
}

func (d delayedReader) Read(p []byte) (n int, err error) {
	time.Sleep(d.delay)
	return d.r.Read(p)
}

func (d delayedReader) Close() error {
	return d.r.Close()
}

type noopWriteCloser struct {
	r io.ReadCloser
}

func newNoopWriteCloser(r io.ReadCloser) noopWriteCloser {
	return noopWriteCloser{r: r}
}

func (r noopWriteCloser) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}

func (w noopWriteCloser) Write(p []byte) (n int, err error) {
	return 0, nil
}

func (w noopWriteCloser) Headers() p2p.Headers {
	return nil
}

func (w noopWriteCloser) ResponseHeaders() p2p.Headers {
	return nil
}

func (w noopWriteCloser) Close() error {
	return w.r.Close()
}

func (w noopWriteCloser) FullClose() error {
	return nil
}

func (w noopWriteCloser) Reset() error {
	return nil
}

type noopReadCloser struct {
	w io.WriteCloser
}

func newNoopReadCloser(w io.WriteCloser) noopReadCloser {
	return noopReadCloser{w: w}
}

func (r noopReadCloser) Read(p []byte) (n int, err error) {
	return 0, nil
}

func (r noopReadCloser) Write(p []byte) (n int, err error) {
	return r.w.Write(p)
}

func (r noopReadCloser) Headers() p2p.Headers {
	return nil
}

func (r noopReadCloser) ResponseHeaders() p2p.Headers {
	return nil
}

func (r noopReadCloser) Close() error {
	return r.w.Close()
}

func (r noopReadCloser) FullClose() error {
	return nil
}

func (r noopReadCloser) Reset() error {
	return nil
}
