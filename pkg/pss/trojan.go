// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pss

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	random "math/rand"

	"github.com/btcsuite/btcd/btcec"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/encryption/elgamal"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrPayloadTooBig is returned when a given payload for a Message type is longer than the maximum amount allowed
	ErrPayloadTooBig = fmt.Errorf("message payload size cannot be greater than %d bytes", MaxPayloadSize)

	// ErrEmptyTargets is returned when the given target list for a trojan chunk is empty
	ErrEmptyTargets = errors.New("target list cannot be empty")

	// ErrVarLenTargets is returned when the given target list for a trojan chunk has addresses of different lengths
	ErrVarLenTargets = errors.New("target list cannot have targets of different length")
)

// Topic is the type that classifies messages, allows client applications to subscribe to
type Topic [32]byte

// NewTopic creates a new Topic from an input string by taking its hash
func NewTopic(text string) Topic {
	bytes, _ := crypto.LegacyKeccak256([]byte(text))
	var topic Topic
	copy(topic[:], bytes[:32])
	return topic
}

// Target is an alias for a partial address (overlay prefix) serving as potential destination
type Target []byte

// Targets is an alias for a collection of targets
type Targets []Target

const (
	// MaxPayloadSize is the maximum allowed payload size for the Message type, in bytes
	MaxPayloadSize = swarm.ChunkSize - 3*swarm.HashSize
)

// Wrap creates a new serialised message with the given topic, payload and recipient public key used
// for encryption
// - span as topic hint  (H(key|topic)[0:8]) to match topic
// chunk payload:
// - nonce is chosen so that the chunk address will have one of the targets as its prefix and thus will be forwarded to the neighbourhood of the recipient overlay address the target is derived from
// trojan payload:
// - ephemeral  public key for el-Gamal encryption
// ciphertext - plaintext:
// - plaintext length encoding
// - integrity protection
// message:
func Wrap(ctx context.Context, topic Topic, msg []byte, recipient *ecdsa.PublicKey, targets Targets) (swarm.Chunk, error) {
	if len(msg) > MaxPayloadSize {
		return nil, ErrPayloadTooBig
	}

	// integrity protection and plaintext msg length encoding
	integrity, err := crypto.LegacyKeccak256(msg)
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint16(integrity[:2], uint16(len(msg)))

	// integrity segment prepended to msg
	plaintext := append(integrity, msg...)
	// use el-Gamal with ECDH on an ephemeral key, recipient public key and topic as salt
	enc, ephpub, err := elgamal.NewEncryptor(recipient, topic[:], 4032, swarm.NewHasher)
	if err != nil {
		return nil, err
	}
	ciphertext, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}

	// prepend serialised ephemeral public key to the ciphertext
	// NOTE: only the random bytes of the compressed public key are used
	// in order not to leak anything, the one bit parity info of the magic byte
	// is encoded in the parity of the 28th byte of the mined nonce
	ephpubBytes := (*btcec.PublicKey)(ephpub).SerializeCompressed()
	payload := append(ephpubBytes[1:], ciphertext...)
	odd := ephpubBytes[0]&0x1 != 0

	if err := checkTargets(targets); err != nil {
		return nil, err
	}
	targetsLen := len(targets[0])

	// topic hash, the first 8 bytes is used as the span of the chunk
	hash, err := crypto.LegacyKeccak256(append(enc.Key(), topic[:]...))
	if err != nil {
		return nil, err
	}
	hint := hash[:8]
	h := hasher(hint, payload)

	// f is evaluating the mined nonce
	// it accepts the nonce if it has the parity required by the ephemeral public key  AND
	// the chunk hashes to an address matching one of the targets
	f := func(nonce []byte) (swarm.Chunk, error) {
		hash, err := h(nonce)
		if err != nil {
			return nil, err
		}
		if !contains(targets, hash[:targetsLen]) {
			return nil, nil
		}
		chunk := swarm.NewChunk(swarm.NewAddress(hash), append(hint, append(nonce, payload...)...))
		return chunk, nil
	}
	return mine(ctx, odd, f)
}

// Unwrap takes a chunk, a topic and a private key, and tries to decrypt the payload
// using the private key, the prepended ephemeral public key for el-Gamal using the topic as salt
func Unwrap(ctx context.Context, key *ecdsa.PrivateKey, chunk swarm.Chunk, topics []Topic) (topic Topic, msg []byte, err error) {
	chunkData := chunk.Data()
	pubkey, err := extractPublicKey(chunkData)
	if err != nil {
		return Topic{}, nil, err
	}
	hint := chunkData[:8]
	for _, topic = range topics {
		select {
		case <-ctx.Done():
			return Topic{}, nil, ctx.Err()
		default:
		}
		dec, err := matchTopic(key, pubkey, hint, topic[:])
		if err != nil {
			privk := crypto.Secp256k1PrivateKeyFromBytes(topic[:])
			dec, err = matchTopic(privk, pubkey, hint, topic[:])
			if err != nil {
				continue
			}
		}
		ciphertext := chunkData[72:]
		msg, err = decryptAndCheck(dec, ciphertext)
		if err != nil {
			continue
		}
		break
	}
	return topic, msg, nil
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
	return func(nonce []byte) ([]byte, error) {
		s := append(nonce, b...)
		hasher := bmtpool.Get()
		defer bmtpool.Put(hasher)
		hasher.SetSpanBytes(span)
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

// mine iteratively enumerates different nonces until the address (BMT hash) of the chunkhas one of the targets as its prefix
func mine(ctx context.Context, odd bool, f func(nonce []byte) (swarm.Chunk, error)) (swarm.Chunk, error) {
	seeds := make([]uint32, 8)
	for i := range seeds {
		seeds[i] = random.Uint32()
	}
	initnonce := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.LittleEndian.PutUint32(initnonce[i*4:i*4+4], seeds[i])
	}
	if odd {
		initnonce[28] |= 0x01
	} else {
		initnonce[28] &= 0xfe
	}
	seeds[7] = binary.LittleEndian.Uint32(initnonce[28:32])

	quit := make(chan struct{})
	// make both  errs  and result channels buffered so they never block
	result := make(chan swarm.Chunk, 8)
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

// extracts ephemeral public key from the chunk data to use with el-Gamal
func extractPublicKey(chunkData []byte) (*ecdsa.PublicKey, error) {
	pubkeyBytes := make([]byte, 33)
	pubkeyBytes[0] |= 0x2
	copy(pubkeyBytes[1:], chunkData[40:72])
	if chunkData[36]|0x1 != 0 {
		pubkeyBytes[0] |= 0x1
	}
	pubkey, err := btcec.ParsePubKey(pubkeyBytes, btcec.S256())
	return (*ecdsa.PublicKey)(pubkey), err
}

// topic is needed to decrypt the trojan payload, but no need to perform decryption with each
// instead the hash of the secret key and the topic is matched against a hint (64 bit meta info)q
// proper integrity check will disambiguate any potential collisions (false positives)
// if the topic matches the hint, it returns the el-Gamal decryptor, otherwise an error
func matchTopic(key *ecdsa.PrivateKey, pubkey *ecdsa.PublicKey, hint []byte, topic []byte) (encryption.Decrypter, error) {
	dec, err := elgamal.NewDecrypter(key, pubkey, topic, swarm.NewHasher)
	if err != nil {
		return nil, err
	}
	match, err := crypto.LegacyKeccak256(append(dec.Key(), topic...))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(hint, match[:8]) {
		return nil, errors.New("topic does not match hint")
	}
	return dec, nil
}

// decrypts the ciphertext with an el-Gamal decryptor using a topic that matched the hint
// the msg is extracted from the plaintext and its integrity is checked
func decryptAndCheck(dec encryption.Decrypter, ciphertext []byte) ([]byte, error) {
	plaintext, err := dec.Decrypt(ciphertext)
	if err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(plaintext[:2]))
	if length > MaxPayloadSize {
		return nil, errors.New("invalid length")
	}
	msg := plaintext[32 : 32+length]
	integrity := plaintext[2:32]
	hash, err := crypto.LegacyKeccak256(msg)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(integrity, hash[2:]) {
		return nil, errors.New("invalid message")
	}
	// bingo
	return msg, nil
}

// ParseRecipient extract ephemeral public key from the hexadecimal string to use with el-Gamal.
func ParseRecipient(recipientHexString string) (*ecdsa.PublicKey, error) {
	publicKeyBytes, err := hex.DecodeString(recipientHexString)
	if err != nil {
		return nil, err
	}
	pubkey, err := btcec.ParsePubKey(publicKeyBytes, btcec.S256())
	if err != nil {
		return nil, err
	}
	return (*ecdsa.PublicKey)(pubkey), err
}
