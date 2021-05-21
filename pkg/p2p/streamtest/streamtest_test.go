// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package streamtest_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/sync/errgroup"
)

func TestRecorder(t *testing.T) {
	var answers = map[string]string{
		"What is your name?":                                    "Sir Lancelot of Camelot",
		"What is your quest?":                                   "To seek the Holy Grail.",
		"What is your favorite color?":                          "Blue.",
		"What is the air-speed velocity of an unladen swallow?": "What do you mean? An African or European swallow?",
	}

	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
				for {
					q, err := rw.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							break
						}
						return fmt.Errorf("read: %w", err)
					}
					q = strings.TrimRight(q, "\n")
					if _, err = rw.WriteString(answers[q] + "\n"); err != nil {
						return fmt.Errorf("write: %w", err)
					}
					if err := rw.Flush(); err != nil {
						return fmt.Errorf("flush: %w", err)
					}
				}
				return nil
			}),
		),
	)

	ask := func(ctx context.Context, s p2p.Streamer, address swarm.Address, questions ...string) (answers []string, err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return nil, fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		for _, q := range questions {
			if _, err := rw.WriteString(q + "\n"); err != nil {
				return nil, fmt.Errorf("write: %w", err)
			}
			if err := rw.Flush(); err != nil {
				return nil, fmt.Errorf("flush: %w", err)
			}

			a, err := rw.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("read: %w", err)
			}
			a = strings.TrimRight(a, "\n")
			answers = append(answers, a)
		}
		return answers, nil
	}

	questions := []string{"What is your name?", "What is your quest?", "What is your favorite color?"}

	aa, err := ask(context.Background(), recorder, swarm.ZeroAddress, questions...)
	if err != nil {
		t.Fatal(err)
	}

	for i, q := range questions {
		if aa[i] != answers[q] {
			t.Errorf("got answer %q for question %q, want %q", aa[i], q, answers[q])
		}
	}

	_, err = recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, "invalid stream name")
	if err != streamtest.ErrRecordsNotFound {
		t.Errorf("got error %v, want %v", err, streamtest.ErrRecordsNotFound)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"What is your name?\nWhat is your quest?\nWhat is your favorite color?\n",
			"Sir Lancelot of Camelot\nTo seek the Holy Grail.\nBlue.\n",
		},
	}, nil)
}

func TestRecorder_errStreamNotSupported(t *testing.T) {
	r := streamtest.New()

	_, err := r.NewStream(context.Background(), swarm.ZeroAddress, nil, "testing", "messages", "1.0.1")
	if !errors.Is(err, streamtest.ErrStreamNotSupported) {
		t.Fatalf("got error %v, want %v", err, streamtest.ErrStreamNotSupported)
	}
}

func TestRecorder_fullcloseWithRemoteClose(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				defer stream.Close()
				_, err := bufio.NewReader(stream).ReadString('\n')
				return err
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
		if _, err := rw.WriteString("message\n"); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := rw.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		return stream.FullClose()
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"message\n",
		},
	}, nil)
}

func TestRecorder_fullcloseWithoutRemoteClose(t *testing.T) {
	streamtest.SetFullCloseTimeout(500 * time.Millisecond)
	defer streamtest.ResetFullCloseTimeout()
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				// don't close the stream here to initiate timeout
				// just try to read the message that it terminated with
				// a new line character
				_, err := bufio.NewReader(stream).ReadString('\n')
				return err
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
		if _, err := rw.WriteString("message\n"); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := rw.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		return stream.FullClose()
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != streamtest.ErrStreamFullcloseTimeout {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"message\n",
		},
	}, nil)
}

func TestRecorder_multipleParallelFullCloseAndClose(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				if _, err := bufio.NewReader(stream).ReadString('\n'); err != nil {
					return err
				}

				var g errgroup.Group
				g.Go(stream.Close)
				g.Go(stream.FullClose)

				if err := g.Wait(); err != nil {
					return err
				}

				return stream.FullClose()
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
		if _, err := rw.WriteString("message\n"); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := rw.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		var g errgroup.Group
		g.Go(stream.Close)
		g.Go(stream.FullClose)

		if err := g.Wait(); err != nil {
			return err
		}

		return nil
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"message\n",
		},
	}, nil)
}

func TestRecorder_closeAfterPartialWrite(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				// just try to read the message that it terminated with
				// a new line character
				_, err := bufio.NewReader(stream).ReadString('\n')
				return err
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		// write a message, but do not write a new line character for handler to
		// know that it is complete
		if _, err := rw.WriteString("unterminated message"); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := rw.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		// deliberately close the stream before the new line character is
		// written to the stream
		if err := stream.Close(); err != nil {
			return err
		}

		// stream should be closed and write should return err
		if _, err := rw.WriteString("expect err message"); err != nil {
			return fmt.Errorf("write: %w", err)
		}

		if err := rw.Flush(); err == nil {
			return fmt.Errorf("expected err")
		}

		return nil
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"unterminated message",
			"",
		},
	}, nil)
}

func TestRecorder_resetAfterPartialWrite(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				// just try to read the message that it terminated with
				// a new line character
				_, err := bufio.NewReader(stream).ReadString('\n')
				return err
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		// write a message, but do not write a new line character for handler to
		// know that it is complete
		if _, err := rw.WriteString("unterminated message"); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if err := rw.Flush(); err != nil {
			return fmt.Errorf("flush: %w", err)
		}

		// deliberately reset the stream before the new line character is
		// written to the stream
		if err := stream.Reset(); err != nil {
			return err
		}

		// stream should be closed and read should return EOF
		if _, err := rw.ReadString('\n'); err != io.EOF {
			return fmt.Errorf("got error %v, want %v", err, io.EOF)
		}

		return nil
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"unterminated message",
			"",
		},
	}, nil)
}

func TestRecorder_withMiddlewares(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}

				if _, err := rw.WriteString("handler, "); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return nil
			}),
		),
		streamtest.WithMiddlewares(
			func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
					if err := h(ctx, peer, stream); err != nil {
						return err
					}
					// close stream after all previous middlewares wrote to it
					// so that the receiving peer can get all the post messages
					return stream.Close()
				}
			},
			func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
					if _, err := stream.Write([]byte("pre 1, ")); err != nil {
						return err
					}
					if err := h(ctx, peer, stream); err != nil {
						return err
					}
					if _, err := stream.Write([]byte("post 1, ")); err != nil {
						return err
					}
					return nil
				}
			},
			func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
					if _, err := stream.Write([]byte("pre 2, ")); err != nil {
						return err
					}
					if err := h(ctx, peer, stream); err != nil {
						return err
					}
					if _, err := stream.Write([]byte("post 2, ")); err != nil {
						return err
					}
					return nil
				}
			},
		),
		streamtest.WithMiddlewares(
			func(h p2p.HandlerFunc) p2p.HandlerFunc {
				return func(ctx context.Context, peer p2p.Peer, stream p2p.Stream) error {
					if _, err := stream.Write([]byte("pre 3, ")); err != nil {
						return err
					}
					if err := h(ctx, peer, stream); err != nil {
						return err
					}
					if _, err := stream.Write([]byte("post 3, ")); err != nil {
						return err
					}
					return nil
				}
			},
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) error {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		if _, err := rw.WriteString("test\n"); err != nil {
			return err
		}
		if err := rw.Flush(); err != nil {
			return err
		}
		_, err = ioutil.ReadAll(rw)
		return err
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"test\n",
			"pre 1, pre 2, pre 3, handler, post 3, post 2, post 1, ",
		},
	}, nil)
}

func TestRecorder_recordErr(t *testing.T) {
	testErr := errors.New("test error")

	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))
				defer stream.Close()

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}

				if _, err := rw.WriteString("resp\n"); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return testErr
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		if _, err = stream.Write([]byte("req\n")); err != nil {
			return err
		}

		_, err = ioutil.ReadAll(stream)
		return err
	}

	err := request(context.Background(), recorder, swarm.ZeroAddress)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"req\n",
			"resp\n",
		},
	}, testErr)
}

func TestRecorder_withPeerProtocols(t *testing.T) {
	peer1 := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("2000000000000000000000000000000000000000000000000000000000000000")
	recorder := streamtest.New(
		streamtest.WithPeerProtocols(map[string]p2p.ProtocolSpec{
			peer1.String(): newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}
				if _, err := rw.WriteString("handler 1\n"); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return nil
			}),
			peer2.String(): newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}
				if _, err := rw.WriteString("handler 2\n"); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return nil
			}),
		}),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) error {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		if _, err := rw.WriteString("req\n"); err != nil {
			return err
		}
		if err := rw.Flush(); err != nil {
			return err
		}
		_, err = rw.ReadString('\n')
		return err
	}

	err := request(context.Background(), recorder, peer1)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peer1, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"req\n",
			"handler 1\n",
		},
	}, nil)

	err = request(context.Background(), recorder, peer2)
	if err != nil {
		t.Fatal(err)
	}

	records, err = recorder.Records(peer2, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"req\n",
			"handler 2\n",
		},
	}, nil)
}

func TestRecorder_withStreamError(t *testing.T) {
	peer1 := swarm.MustParseHexAddress("1000000000000000000000000000000000000000000000000000000000000000")
	peer2 := swarm.MustParseHexAddress("2000000000000000000000000000000000000000000000000000000000000000")
	testErr := errors.New("dummy stream error")
	recorder := streamtest.New(
		streamtest.WithPeerProtocols(map[string]p2p.ProtocolSpec{
			peer1.String(): newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}
				if _, err := rw.WriteString("handler 1\n"); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return nil
			}),
			peer2.String(): newTestProtocol(func(_ context.Context, peer p2p.Peer, stream p2p.Stream) error {
				rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

				if _, err := rw.ReadString('\n'); err != nil {
					return err
				}
				if _, err := rw.WriteString("handler 2\n"); err != nil {
					return err
				}
				if err := rw.Flush(); err != nil {
					return err
				}

				return nil
			}),
		}),
		streamtest.WithStreamError(func(addr swarm.Address, _, _, _ string) error {
			if addr.String() == peer1.String() {
				return testErr
			}
			return nil
		}),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) error {
		stream, err := s.NewStream(ctx, address, nil, testProtocolName, testProtocolVersion, testStreamName)
		if err != nil {
			return fmt.Errorf("new stream: %w", err)
		}
		defer stream.Close()

		rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

		if _, err := rw.WriteString("req\n"); err != nil {
			return err
		}
		if err := rw.Flush(); err != nil {
			return err
		}
		_, err = rw.ReadString('\n')
		return err
	}

	err := request(context.Background(), recorder, peer1)
	if err == nil {
		t.Fatal("expected error on NewStream for peer")
	}

	err = request(context.Background(), recorder, peer2)
	if err != nil {
		t.Fatal(err)
	}

	records, err := recorder.Records(peer2, testProtocolName, testProtocolVersion, testStreamName)
	if err != nil {
		t.Fatal(err)
	}

	testRecords(t, records, [][2]string{
		{
			"req\n",
			"handler 2\n",
		},
	}, nil)
}

const (
	testProtocolName    = "testing"
	testProtocolVersion = "1.0.1"
	testStreamName      = "messages"
)

func newTestProtocol(h p2p.HandlerFunc) p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    testProtocolName,
		Version: testProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    testStreamName,
				Handler: h,
			},
		},
	}
}

func testRecords(t *testing.T, records []*streamtest.Record, want [][2]string, wantErr error) {
	t.Helper()

	lr := len(records)
	lw := len(want)
	if lr != lw {
		t.Fatalf("got %v records, want %v", lr, lw)
	}

	for i := 0; i < lr; i++ {
		record := records[i]

		if err := record.Err(); err != wantErr {
			t.Fatalf("got error from record %v, want %v", err, wantErr)
		}

		w := want[i]

		gotIn := string(record.In())
		if gotIn != w[0] {
			t.Errorf("got stream in %q, want %q", gotIn, w[0])
		}

		gotOut := string(record.Out())
		if gotOut != w[1] {
			t.Errorf("got stream out %q, want %q", gotOut, w[1])
		}
	}
}
