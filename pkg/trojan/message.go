// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trojan

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"time"

	bmtlegacy "github.com/ethersphere/bmt/legacy"

	"github.com/ethersphere/bee/pkg/swarm"
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
	NonceSize      = 32
	LengthSize     = 2
	TopicSize      = 32
	MinerTimeout   = 5 // seconds after which the mining will fail
)

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
func (m *Message) Wrap(targets Targets) (swarm.Chunk, error) {
	if err := checkTargets(targets); err != nil {
		return nil, err
	}

	span := make([]byte, 8)
	binary.LittleEndian.PutUint64(span, swarm.ChunkSize)
	return m.toChunk(targets, span)
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
	// chunk must be content-addressed to be trojan
	if c.Type() != swarm.ContentAddressed {
		return false
	}

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

// toChunk finds a nonce so that when the given trojan chunk fields are hashed, the result will fall in the neighbourhood of one of the given targets
// this is done by iteratively enumerating different nonces until the BMT hash of the serialization of the trojan chunk fields results in a chunk address that has one of the targets as its prefix
// the function returns a new chunk, with the found matching hash to be used as its address,
// and its data set to the serialization of the trojan chunk fields which correctly hash into the matching address
func (m *Message) toChunk(targets Targets, span []byte) (swarm.Chunk, error) {
	// start out with random nonce
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	nonceInt := new(big.Int).SetBytes(nonce)
	targetsLen := len(targets[0])

	// serialize message
	b, err := m.MarshalBinary() // TODO: this should be encrypted
	if err != nil {
		return nil, err
	}

	errC := make(chan error)
	var hash, s []byte
	go func() {
		defer close(errC)

		// mining operation: hash chunk fields with different nonces until an acceptable one is found
		for {
			s = append(append(span, nonce...), b...) // serialize chunk fields
			hash, err = hashBytes(s)
			if err != nil {
				errC <- err
				return
			}

			// take as much of the hash as the targets are long
			if contains(targets, hash[:targetsLen]) {
				// if nonce found, stop loop and return chunk
				errC <- nil
				return
			}
			// else, add 1 to nonce and try again
			nonceInt.Add(nonceInt, big.NewInt(1))
			// loop around in case of overflow after 256 bits
			if nonceInt.BitLen() > (NonceSize * swarm.SpanSize) {
				nonceInt = big.NewInt(0)
			}
			nonce = padBytesLeft(nonceInt.Bytes()) // pad in case Bytes call is not 32 bytes long
		}
	}()

	// checks whether the mining is completed or times out
	select {
	case err := <-errC:
		if err == nil {
			return swarm.NewChunk(swarm.NewAddress(hash), s), nil
		}
		return nil, err
	case <-time.After(MinerTimeout * time.Second):
		return nil, ErrMinerTimeout
	}
}

// hashBytes hashes the given serialization of chunk fields with the hashing func
func hashBytes(s []byte) ([]byte, error) {
	hashPool := bmtlegacy.NewTreePool(swarm.NewHasher, swarm.Branches, bmtlegacy.PoolSize)
	hasher := bmtlegacy.New(hashPool)
	hasher.Reset()
	span := binary.LittleEndian.Uint64(s[:8])
	err := hasher.SetSpan(int64(span))
	if err != nil {
		return nil, err
	}
	if _, err := hasher.Write(s[8:]); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
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

// padBytesLeft adds 0s to the given byte slice as left padding,
// returning this as a new byte slice with a length of exactly 32
// given param is assumed to be at most 32 bytes long
func padBytesLeft(b []byte) []byte {
	l := len(b)
	if l == 32 {
		return b
	}
	bb := make([]byte, 32)
	copy(bb[32-l:], b)
	return bb
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
