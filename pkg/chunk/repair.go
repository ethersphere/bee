// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunk

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

const (
	// RecoveryTopicText is the string used to construct the recovery topic
	RecoveryTopicText = "RECOVERY"
)

var (
	// RecoveryTopic is the topic used for repairing globally pinned chunks
	RecoveryTopic = trojan.NewTopic(RecoveryTopicText)

	// ErrPublisher is returned when the publisher address turns out to be empty
	ErrPublisher = errors.New("content publisher is empty")

	// ErrPubKey is returned when the publisher bytes cannot be decompressed as a public key
	ErrPubKey = errors.New("failed to decompress public key")

	// ErrFeedLookup is returned when the recovery feed cannot be successefully looked up
	ErrFeedLookup = errors.New("failed to look up recovery feed")

	// ErrFeedContent is returned when there is a failure to retrieve content from the recovery feed
	ErrFeedContent = errors.New("failed to get content for recovery feed")

	// ErrTargets is returned when there is a failure to unmarshal the feed content as a trojan.Targets variable
	ErrTargets = errors.New("failed to unmarshal targets in recovery feed content")
)

// RecoveryHook defines code to be executed upon failing to retrieve pinned chunks
type RecoveryHook func(ctx context.Context, chunkAddress swarm.Address) error

// sender is the function call for sending trojan chunks
type sender func(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*pss.Monitor, error)

// NewRecoveryHook returns a new RecoveryHook with the sender function defined
func NewRecoveryHook(send sender) RecoveryHook {
	return func(ctx context.Context, chunkAddress swarm.Address) error {
		targets, err := getPinners(ctx)
		if err != nil {
			return err
		}
		payload := chunkAddress

		// TODO: returned monitor should be made use of
		if _, err := send(ctx, targets, RecoveryTopic, payload); err != nil {
			return err
		}
		return nil
	}
}

// NewRepairHandler creates a repair function to re-upload globally pinned chunks to the network with the given store
func NewRepairHandler(s *chunk.ValidatorStore) pss.Handler {
	return func(m trojan.Message) {
		chAddr := m.Payload
		err := s.Set(context.Background(), storage.ModeSetReUpload, chAddr)
	}
}

// getPinners returns the specific target pinners for a corresponding chunk
func getPinners(ctx context.Context) (trojan.Targets, error) {

	targets := new(trojan.Targets)

	return *targets, nil
}