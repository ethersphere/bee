// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trojan

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	random "math/rand"

	"github.com/ethersphere/bee/pkg/swarm"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
)

// Topic is an alias for a 32 byte fixed-size array which contains an encoding of a message topic
type Topic [32]byte

// Target is an alias for an address which can be mined to construct a trojan message.
// Target is like partial address which helps to send message to a particular PO.
type Target []byte

// Targets is an alias for a collection of targets
type Targets []Target

// Message represents a trojan message, which is a message that will be hidden within a chunk payload as part of its data
type Message struct {
	length  [2]byte // big-endian encoding of Message payload length
	Topic   Topic
	Payload []byte // contains the chunk address to be repaired
	padding []byte
}

const (
	// MaxPayloadSize is the maximum allowed payload size for the Message type, in bytes
	// MaxPayloadSize + Topic + Length + Nonce = Default ChunkSize
	//    (4030)      +  (32) +   (2)  +  (32) = 4096 Bytes
	MaxPayloadSize = swarm.ChunkSize - NonceSize - LengthSize - TopicSize
	// NonceSize is a hash bit sequence
	NonceSize = 32
	// LengthSize is the byte length to represent message
	LengthSize = 2
	// TopicSize is a hash bit sequence
	TopicSize = 32
)

// var MinerTimeout = 20 * time.Second

// NewTopic creates a new Topic variable with the given input string
// the input string is taken as a byte slice and hashed
func NewTopic(topic string) Topic {
	var tpc Topic
	hasher := swarm.NewHasher()
	_, err := hasher.Write([]byte(topic))
	if err != nil {
		return tpc
	}
	sum := hasher.Sum(nil)
	copy(tpc[:], sum)
	return tpc
}

// NewMessage creates a new Message variable with the given topic and payload
// it finds a length and nonce for the message according to the given input and maximum payload size
func NewMessage(topic Topic, payload []byte) (Message, error) {
	if len(payload) > MaxPayloadSize {
		return Message{}, ErrPayloadTooBig
	}

	// get length as array of 2 bytes
	payloadSize := uint16(len(payload))

	// set random bytes as padding
	paddingLen := MaxPayloadSize - payloadSize
	padding := make([]byte, paddingLen)
	if _, err := rand.Read(padding); err != nil {
		return Message{}, err
	}

	// create new Message var and set fields
	m := new(Message)
	binary.BigEndian.PutUint16(m.length[:], payloadSize)
	m.Topic = topic
	m.Payload = payload
	m.padding = padding
	return *m, nil
}

// Wrap creates a new trojan chunk for the given targets and Message
// a trojan chunk is a content-addressed chunk made up of span, a nonce, and a payload which contains the Message
// the chunk address will have one of the targets as its prefix and thus will be forwarded to the neighbourhood of the recipient overlay address the target is derived from
// this is done by iteratively enumerating different nonces until the BMT hash of the serialization of the trojan chunk fields results in a chunk address that has one of the targets as its prefix
func (m *Message) Wrap(ctx context.Context, targets Targets) (swarm.Chunk, error) {
	if err := checkTargets(targets); err != nil {
		return nil, err
	}
	targetsLen := len(targets[0])

	// serialize message
	b, err := m.MarshalBinary() // TODO: this should be encrypted
	if err != nil {
		return nil, err
	}
	span := make([]byte, 8)
	binary.LittleEndian.PutUint64(span, uint64(len(b)+NonceSize))
	h := hasher(span, b)
	f := func(nonce []byte) (interface{}, error) {
		hash, err := h(nonce)
		if err != nil {
			return nil, err
		}
		if !contains(targets, hash[:targetsLen]) {
			return nil, nil
		}
		chunk := swarm.NewChunk(swarm.NewAddress(hash), append(span, append(nonce, b...)...))
		return chunk, nil
	}
	chunk, err := mine(ctx, f)
	if err != nil {
		return nil, err
	}
	return chunk.(swarm.Chunk), nil
}

// Unwrap creates a new trojan message from the given chunk payload
// this function assumes the chunk has been validated as a content-addressed chunk
// it will return the resulting message if the unwrapping is successful, and an error otherwise
func Unwrap(c swarm.Chunk) (*Message, error) {
	d := c.Data()

	// unmarshal chunk payload into message
	m := new(Message)
	// first 40 bytes are span + nonce
	err := m.UnmarshalBinary(d[40:])

	return m, err
}

// IsPotential returns true if the given chunk is a potential trojan
func IsPotential(c swarm.Chunk) bool {
	data := c.Data()
	// check for minimum chunk data length
	trojanChunkMinDataLen := swarm.SpanSize + NonceSize + TopicSize + LengthSize
	if len(data) < trojanChunkMinDataLen {
		return false
	}

	// check for valid trojan message length in bytes #41 and #42
	messageLen := int(binary.BigEndian.Uint16(data[40:42]))
	return trojanChunkMinDataLen+messageLen <= len(data)
}

// checkTargets verifies that the list of given targets is non empty and with elements of matching size
func checkTargets(targets Targets) error {
	if len(targets) == 0 {
		return ErrEmptyTargets
	}
	validLen := len(targets[0]) // take first element as allowed length
	for i := 1; i < len(targets); i++ {
		if len(targets[i]) != validLen {
			return ErrVarLenTargets
		}
	}
	return nil
}

func hasher(span, b []byte) func([]byte) ([]byte, error) {
	hashPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	return func(nonce []byte) ([]byte, error) {
		s := append(nonce, b...) // serialize chunk fields
		hasher := bmtlegacy.New(hashPool)
		if err := hasher.SetSpanBytes(span); err != nil {
			return nil, err
		}
		if _, err := hasher.Write(s); err != nil {
			return nil, err
		}
		return hasher.Sum(nil), nil
	}
}

// contains returns whether the given collection contains the given element
func contains(col Targets, elem []byte) bool {
	for i := range col {
		if bytes.Equal(elem, col[i]) {
			return true
		}
	}
	return false
}

// MarshalBinary serializes a message struct
func (m *Message) MarshalBinary() (data []byte, err error) {
	data = append(m.length[:], m.Topic[:]...)
	data = append(data, m.Payload...)
	data = append(data, m.padding...)
	return data, nil
}

// UnmarshalBinary deserializes a message struct
func (m *Message) UnmarshalBinary(data []byte) (err error) {
	if len(data) < LengthSize+TopicSize {
		return ErrUnmarshal
	}

	copy(m.length[:], data[:LengthSize])                    // first 2 bytes are length
	copy(m.Topic[:], data[LengthSize:LengthSize+TopicSize]) // following 32 bytes are topic

	length := binary.BigEndian.Uint16(m.length[:])
	if (len(data) - LengthSize - TopicSize) < int(length) {
		return ErrUnmarshal
	}

	// rest of the bytes are payload and padding
	payloadEnd := LengthSize + TopicSize + length
	m.Payload = data[LengthSize+TopicSize : payloadEnd]
	m.padding = data[payloadEnd:]
	return nil
}

func mine(ctx context.Context, f func(nonce []byte) (interface{}, error)) (interface{}, error) {
	seeds := make([]uint32, 8)
	for i := range seeds {
		seeds[i] = random.Uint32()
	}
	initnonce := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.LittleEndian.PutUint32(initnonce[i*4:i*4+4], seeds[i])
	}
	quit := make(chan struct{})
	// make both  errs  and result channels buffered so they never block
	result := make(chan interface{}, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(j int) {
			nonce := make([]byte, 32)
			copy(nonce, initnonce)
			for seed := seeds[j]; ; seed++ {
				binary.LittleEndian.PutUint32(nonce[j*4:j*4+4], seed)
				res, err := f(nonce)
				if err != nil {
					errs <- err
					return
				}
				if res != nil {
					result <- res
					return
				}
				select {
				case <-quit:
					return
				default:
				}
			}
		}(i)
	}
	defer close(quit)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errs:
		return nil, err
	case res := <-result:
		return res, nil
	}
}
