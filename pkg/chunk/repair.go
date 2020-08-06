// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunk

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

const (
	// RecoveryTopicText is the string used to construct the recovery topic.
	RecoveryTopicText = "RECOVERY"
)

var (
	// RecoveryTopic is the topic used for repairing globally pinned chunks.
	RecoveryTopic = trojan.NewTopic(RecoveryTopicText)

	// ErrTargetPrefix is returned when target prefix decoding fails.
	ErrTargetPrefix = errors.New("error decoding prefix string")
)

// RecoveryHook defines code to be executed upon failing to retrieve chunks.
type RecoveryHook func(ctx context.Context, chunkAddress swarm.Address) error

// sender is the function call for sending trojan chunks.
type sender func(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) (*pss.Monitor, error)

// NewRecoveryHook returns a new RecoveryHook with the sender function defined.
func NewRecoveryHook(send sender, logger logging.Logger) RecoveryHook {
	return func(ctx context.Context, chunkAddress swarm.Address) error {
		targets, err := getPinners(ctx, logger)
		if err != nil {
			return err
		}
		payload := chunkAddress

		if _, err := send(ctx, targets, RecoveryTopic, payload.Bytes()); err != nil {
			return err
		}
		return nil
	}
}

// NewRepairHandler creates a repair function to re-upload globally pinned chunks to the network with the given store.
func NewRepairHandler(s storage.Storer, logger logging.Logger, pushSyncer pushsync.PushSyncer) pss.Handler {
	return func(m trojan.Message) {
		chAddr := m.Payload
		ctx := context.Background()

		ch, err := s.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(chAddr))
		if err != nil {
			logger.Debugf("chunk repair: error while getting chunk for repairing: %v", err)
			return
		}

		// push the chunk using push sync so that it reaches it destination in network
		_, err = pushSyncer.PushChunkToClosest(ctx, ch)
		if err != nil {
			logger.Debugf("chunk repair: error while sending chunk or receiving receipt: %v", err)
			return
		}
	}
}

// getPinners returns the specific target pinners for a corresponding chunk by
// reading the prefix targets sent in the download API.
func getPinners(ctx context.Context, logger logging.Logger) (trojan.Targets, error) {
	targetString := sctx.GetTargets(ctx)
	prefixes := strings.Split(targetString, ",")

	var targets trojan.Targets
	for _, prefix := range prefixes {
		var target trojan.Target
		target, err := hex.DecodeString(prefix)
		if err != nil {
			logger.Errorf("error decoding target prefix: %s", prefix)
			continue
		}
		targets = append(targets, target)
	}
	return targets, nil
}
