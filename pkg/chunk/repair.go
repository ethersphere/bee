// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunk

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/logging"
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

	// ErrTargetPrefix is returned when target prefix decoding fails
	ErrTargetPrefix = errors.New("error decoding prefix string")
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
		if _, err := send(ctx, targets, RecoveryTopic, payload.Bytes()); err != nil {
			return err
		}
		return nil
	}
}

// NewRepairHandler creates a repair function to re-upload globally pinned chunks to the network with the given store
func NewRepairHandler(s storage.Storer, logger logging.Logger) pss.Handler {
	return func(m trojan.Message) {
		chAddr := m.Payload
		err := s.Set(context.Background(), storage.ModeSetReUpload, swarm.NewAddress(chAddr))
		if err != nil {
			logger.Debugf("chunk repair: could not set ModeSetReUpload: %v", err)
		}
	}
}

// getPinners returns the specific target pinners for a corresponding chunk by
// reading the prefix targets sent in the download API
func getPinners(ctx context.Context) (trojan.Targets, error) {
	targetString := ctx.Value(api.TargetsContextKey{}).(string)
	prefixes := strings.Split(targetString, ",")

	var targets trojan.Targets
	for _, prefix := range prefixes {
		var target trojan.Target
		prefix = strings.TrimPrefix(prefix, "0x")
		target, err := hex.DecodeString(prefix)
		if err != nil {
			return nil, ErrTargetPrefix
		}
		targets = append(targets, target)
	}
	return targets, nil
}
