// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package recovery

import (
	"context"
<<<<<<< HEAD
=======
	"errors"
>>>>>>> fix review commenst from Viktor

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/pss"
	"github.com/ethersphere/bee/pkg/pushsync"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/trojan"
)

const (
	// recoveryTopicText is the string used to construct the recovery topic.
	recoveryTopicText = "RECOVERY"
)

var (
	// recoveryTopic is the topic used for repairing globally pinned chunks.
	RecoveryTopic       = trojan.NewTopic(recoveryTopicText)
	defaultTargetLength = 2
<<<<<<< HEAD
=======
)

var (
	errChunkNotPresent = errors.New("chunk repair: chunk not present in local store for repairing")
	errInvalidEnvelope = errors.New("invalid envelope")
>>>>>>> fix review commenst from Viktor
)

// RecoveryHook defines code to be executed upon failing to retrieve chunks.
type RecoveryHook func(ctx context.Context, chunkAddress swarm.Address, targets trojan.Targets, chunkC chan swarm.Chunk) (swarm.Address, error)

// sender is the function call for sending trojan chunks.
type PssSender interface {
	Send(ctx context.Context, targets trojan.Targets, topic trojan.Topic, payload []byte) error
}

// NewRecoveryHook returns a new RecoveryHook with the sender function defined.
func NewRecoveryHook(pss PssSender, overlay swarm.Address, signer crypto.Signer) RecoveryHook {
	return func(ctx context.Context, chunkAddress swarm.Address, targets trojan.Targets, chunkC chan swarm.Chunk) (swarm.Address, error) {
		//create the SOC envelope
		pubKey, err := signer.PublicKey()
		if err != nil {
			return swarm.ZeroAddress, err
		}
		address, err := crypto.NewEthereumAddress(*pubKey)
		if err != nil {
			return swarm.ZeroAddress, err
		}
		owner, err := soc.NewOwner(address)
		if err != nil {
			return swarm.ZeroAddress, err
		}
		socEnvelope, err := soc.CreateSelfAddressedEnvelope(ctx, owner, overlay, defaultTargetLength, chunkAddress, signer)
		if err != nil {
			return swarm.ZeroAddress, err
		}
		payload := append(socEnvelope.Address().Bytes(), socEnvelope.Data()...)

		// Send the recovery PSS message with the self addressed envelope
		err = pss.Send(ctx, targets, RecoveryTopic, payload)
		return socEnvelope.Address(), err
	}
}

// NewRepairHandler creates a repair function to re-upload globally pinned chunks to the network with the given store.
<<<<<<< HEAD
func NewRepairHandler(s storage.Storer, logger logging.Logger, pushSyncer pushsync.PushSyncer) pss.Handler {
	return func(ctx context.Context, m *trojan.Message) {
		if len(m.Payload) != swarm.HashSize+soc.IdSize+soc.SignatureSize+swarm.SpanSize+swarm.HashSize {
			logger.Tracef("chunk repair: invalid payload length: %d", len(m.Payload))
			return
=======
func NewRepairHandler(s storage.Storer, pushSyncer pushsync.PushSyncer) pss.Handler {
	return func(ctx context.Context, m *trojan.Message) error {
		if len(m.Payload) != swarm.HashSize+soc.IdSize+soc.SignatureSize+swarm.SpanSize+swarm.HashSize {
			return errInvalidEnvelope
>>>>>>> fix review commenst from Viktor
		}

		// get the envelopeAddress and the payloadId from the soc envelope
		envelopeAddress, envelopePayloadId := soc.GetAddressAndPayloadId(m.Payload)

		// check if the chunk exists in the local store and proceed.
		// otherwise the Get will trigger a unnecessary network retrieve
		addrToRetrieve := swarm.NewAddress(envelopePayloadId)
		exists, err := s.Has(ctx, addrToRetrieve)
		if err != nil {
			logger.Tracef("chunk repair: %w", err)
			return
		}
		if !exists {
			logger.Tracef("chunk repair: chunk not present to repair: %v", addrToRetrieve.String())
			return
		}

		// retrieve the chunk from the local store
		ch, err := s.Get(ctx, storage.ModeGetRequest, addrToRetrieve)
		if err != nil {
			logger.Tracef("chunk repair: error while getting chunk for repairing: %v", err)
			return
		}

		// push the chunk using push sync so that it reaches it destination in network
		_, err = pushSyncer.PushChunkToClosest(ctx, ch)
		if err != nil {
			logger.Tracef("chunk repair: error while repairing chunk in network: %v", err)
			return
		}

		// construct a new soc chunk from the soc envelope by adding the chunk data to the envelope
		recoveryResponse := newRecoveryResponseChunk(envelopeAddress, m.Payload, ch.Data())

		// send the soc envelope which has the chunk data, to the requester
		_, err = pushSyncer.PushChunkToClosest(ctx, recoveryResponse)
		if err != nil {
<<<<<<< HEAD
			logger.Tracef("chunk repair: error while sending chunk repair response: %v", err)
			return
		}
=======
			return err
		}
		return nil
>>>>>>> fix review commenst from Viktor
	}
}

// newRecoveryResponseChunk creates the proper SOC chunk with the repaired chunk data as payload by using the
// ID and Signature received in the envelope
func newRecoveryResponseChunk(address []byte, payload []byte, chunkData []byte) swarm.Chunk {
	data := make([]byte, soc.IdSize+soc.SignatureSize+len(chunkData))
	cursor := 0
	envelopeCursor := swarm.HashSize
	copy(data[cursor:cursor+soc.IdSize], payload[envelopeCursor:envelopeCursor+soc.IdSize])
	cursor += soc.IdSize
	envelopeCursor += soc.IdSize
	copy(data[cursor:cursor+soc.SignatureSize], payload[envelopeCursor:envelopeCursor+soc.SignatureSize])
	cursor += soc.SignatureSize
	copy(data[cursor:cursor+len(chunkData)], chunkData)
	return swarm.NewChunk(swarm.NewAddress(address), data)
}