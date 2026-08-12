// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/websocket"
)

// feedIndexSize is the width in bytes of the big-endian feed index prefix on
// a FEED_TOPIC publish frame.
const feedIndexSize = 8

// parsePublishFrame decodes one inbound binary WS frame into a signed SOC.
//
// ANCHOR (explicit regimes): sig(65) ‖ span(8) ‖ payload — id = topic bytes.
// FEED_TOPIC: index(8, big-endian) ‖ sig(65) ‖ span(8) ‖ payload — id =
// bps.FeedID(topic, index).
func parsePublishFrame(binding pb.TopicBinding, topic swarm.Address, owner []byte, frame []byte) (*soc.SOC, error) {
	var id []byte
	rest := frame

	switch binding {
	case pb.TopicBinding_ANCHOR:
		if len(rest) < swarm.SocSignatureSize+swarm.SpanSize {
			return nil, fmt.Errorf("bps: publish frame too short for anchor binding")
		}
		id = topic.Bytes()
	case pb.TopicBinding_FEED_TOPIC:
		if len(rest) < feedIndexSize+swarm.SocSignatureSize+swarm.SpanSize {
			return nil, fmt.Errorf("bps: publish frame too short for feed-topic binding")
		}
		index := binary.BigEndian.Uint64(rest[:feedIndexSize])
		fid, err := bps.FeedID(topic.Bytes(), index)
		if err != nil {
			return nil, fmt.Errorf("bps: derive feed id: %w", err)
		}
		id = fid
		rest = rest[feedIndexSize:]
	default:
		return nil, fmt.Errorf("bps: unsupported topic binding %v", binding)
	}

	sig := rest[:swarm.SocSignatureSize]
	spanPayload := rest[swarm.SocSignatureSize:]
	payload := spanPayload[swarm.SpanSize:]
	if len(payload) > swarm.ChunkSize {
		return nil, fmt.Errorf("bps: publish frame payload exceeds chunk size")
	}

	ch, err := cac.NewWithDataSpan(spanPayload)
	if err != nil {
		return nil, fmt.Errorf("bps: assemble wrapped chunk: %w", err)
	}

	sc, err := soc.NewSigned(id, ch, owner, sig)
	if err != nil {
		return nil, fmt.Errorf("bps: assemble soc: %w", err)
	}

	if err := verifyPublishSignature(id, ch.Address(), owner, sig); err != nil {
		return nil, err
	}

	return sc, nil
}

// verifyPublishSignature recovers the signer of id ‖ chunkAddress from sig
// and confirms it matches owner. This is the invalid-signature path.
func verifyPublishSignature(id []byte, chunkAddr swarm.Address, owner, sig []byte) error {
	h := swarm.NewHasher()
	if _, err := h.Write(id); err != nil {
		return fmt.Errorf("bps: hash soc digest: %w", err)
	}
	if _, err := h.Write(chunkAddr.Bytes()); err != nil {
		return fmt.Errorf("bps: hash soc digest: %w", err)
	}
	digest := h.Sum(nil)

	pubKey, err := crypto.Recover(sig, digest)
	if err != nil {
		return fmt.Errorf("bps: invalid signature: %w", err)
	}
	recovered, err := crypto.NewEthereumAddress(*pubKey)
	if err != nil {
		return fmt.Errorf("bps: invalid signature: %w", err)
	}
	if !bytes.Equal(recovered, owner) {
		return fmt.Errorf("bps: invalid signature: owner mismatch")
	}
	return nil
}

// socFields is the parsed swarm-soc-fields header naming which fields of a
// SOC an outbound WS message should carry.
type socFields struct {
	address, recoveredPubKey, identifier, signature, wrappedAddress, span, payload bool
}

// parseSocFields parses the comma-separated swarm-soc-fields header value.
// An empty header selects payload only.
func parseSocFields(header string) (socFields, error) {
	var f socFields

	header = strings.TrimSpace(header)
	if header == "" {
		f.payload = true
		return f, nil
	}

	for _, part := range strings.Split(header, ",") {
		switch strings.TrimSpace(part) {
		case "address":
			f.address = true
		case "recoveredPubKey":
			f.recoveredPubKey = true
		case "identifier":
			f.identifier = true
		case "signature":
			f.signature = true
		case "wrappedAddress":
			f.wrappedAddress = true
		case "span":
			f.span = true
		case "payload":
			f.payload = true
		default:
			return socFields{}, fmt.Errorf("bps: unknown soc field %q", strings.TrimSpace(part))
		}
	}
	return f, nil
}

// serializeSoc renders one outbound message for sc according to f. A
// payload-only selection produces a raw binary frame; any other selection
// produces a JSON text frame with hex-encoded values for exactly the
// requested fields.
func serializeSoc(f socFields, sc *soc.SOC) (msgType int, data []byte, err error) {
	wrapped := sc.WrappedChunk()
	wrappedData := wrapped.Data()
	var span, payload []byte
	if len(wrappedData) >= swarm.SpanSize {
		span = wrappedData[:swarm.SpanSize]
		payload = wrappedData[swarm.SpanSize:]
	} else {
		payload = wrappedData
	}

	if f.payload && !f.address && !f.recoveredPubKey && !f.identifier && !f.signature && !f.wrappedAddress && !f.span {
		return websocket.BinaryMessage, payload, nil
	}

	out := make(map[string]string)

	if f.address {
		addr, err := sc.Address()
		if err != nil {
			return 0, nil, fmt.Errorf("bps: soc address: %w", err)
		}
		out["address"] = hex.EncodeToString(addr.Bytes())
	}
	if f.recoveredPubKey {
		out["recoveredPubKey"] = hex.EncodeToString(sc.OwnerPubKey())
	}
	if f.identifier {
		out["identifier"] = hex.EncodeToString(sc.ID())
	}
	if f.signature {
		out["signature"] = hex.EncodeToString(sc.Signature())
	}
	if f.wrappedAddress {
		out["wrappedAddress"] = hex.EncodeToString(wrapped.Address().Bytes())
	}
	if f.span {
		out["span"] = hex.EncodeToString(span)
	}
	if f.payload {
		out["payload"] = hex.EncodeToString(payload)
	}

	data, err = json.Marshal(out)
	if err != nil {
		return 0, nil, fmt.Errorf("bps: marshal soc fields: %w", err)
	}
	return websocket.TextMessage, data, nil
}
