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
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/pkg/swarm"
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
			newTestProtocol(func(peer p2p.Peer, stream p2p.Stream) error {
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
		stream, err := s.NewStream(ctx, address, testProtocolName, testStreamName, testStreamVersion)
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

	_, err = recorder.Records(swarm.ZeroAddress, testProtocolName, "invalid stream name", testStreamVersion)
	if err != streamtest.ErrRecordsNotFound {
		t.Errorf("got error %v, want %v", err, streamtest.ErrRecordsNotFound)
	}

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testStreamName, testStreamVersion)
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want 1", l)
	}

	record := records[0]

	if err := record.Err(); err != nil {
		t.Fatalf("got error from record %v, want nil", err)
	}

	wantIn := "What is your name?\nWhat is your quest?\nWhat is your favorite color?\n"
	gotIn := string(record.In())
	if gotIn != wantIn {
		t.Errorf("got stream in %q, want %q", gotIn, wantIn)
	}

	wantOut := "Sir Lancelot of Camelot\nTo seek the Holy Grail.\nBlue.\n"
	gotOut := string(record.Out())
	if gotOut != wantOut {
		t.Errorf("got stream out %q, want %q", gotOut, wantOut)
	}
}

func TestRecorder_errStreamNotSupported(t *testing.T) {
	r := streamtest.New()

	_, err := r.NewStream(context.Background(), swarm.ZeroAddress, "testing", "messages", "1.0.1")
	if !errors.Is(err, streamtest.ErrStreamNotSupported) {
		t.Fatalf("got error %v, want %v", err, streamtest.ErrStreamNotSupported)
	}
}

func TestRecorder_closeAfterPartialWrite(t *testing.T) {
	recorder := streamtest.New(
		streamtest.WithProtocols(
			newTestProtocol(func(peer p2p.Peer, stream p2p.Stream) error {
				// just try to read the message that it terminated with
				// a new line character
				_, err := bufio.NewReader(stream).ReadString('\n')
				return err
			}),
		),
	)

	request := func(ctx context.Context, s p2p.Streamer, address swarm.Address) (err error) {
		stream, err := s.NewStream(ctx, address, testProtocolName, testStreamName, testStreamVersion)
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

	records, err := recorder.Records(swarm.ZeroAddress, testProtocolName, testStreamName, testStreamVersion)
	if err != nil {
		t.Fatal(err)
	}

	if l := len(records); l != 1 {
		t.Fatalf("got %v records, want 1", l)
	}

	record := records[0]

	if err := record.Err(); err != nil {
		t.Fatalf("got error from record %v, want nil", err)
	}

	wantIn := "unterminated message"
	gotIn := string(record.In())
	if gotIn != wantIn {
		t.Errorf("got stream in %q, want %q", gotIn, wantIn)
	}

	wantOut := ""
	gotOut := string(record.Out())
	if gotOut != wantOut {
		t.Errorf("got stream out %q, want %q", gotOut, wantOut)
	}
}

const (
	testProtocolName  = "testing"
	testStreamName    = "messages"
	testStreamVersion = "1.0.1"
)

func newTestProtocol(h p2p.HandlerFunc) p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name: testProtocolName,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    testStreamName,
				Version: testStreamVersion,
				Handler: h,
			},
		},
	}
}
