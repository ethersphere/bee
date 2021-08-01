// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mantaray

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	maxUint16 = ^uint16(0)
)

// Version constants.
const (
	versionNameString   = "mantaray"
	versionCode01String = "0.1"
	versionCode02String = "0.2"

	versionSeparatorString = ":"

	version01String     = versionNameString + versionSeparatorString + versionCode01String   // "mantaray:0.1"
	version01HashString = "025184789d63635766d78c41900196b57d7400875ebe4d9b5d1e76bd9652a9b7" // pre-calculated version string, Keccak-256

	version02String     = versionNameString + versionSeparatorString + versionCode02String   // "mantaray:0.2"
	version02HashString = "5768b3b6a7db56d21d1abff40d41cebfc83448fed8d7e9b06ec0d3b073f28f7b" // pre-calculated version string, Keccak-256
)

// Node header fields constants.
const (
	nodeObfuscationKeySize = 32
	versionHashSize        = 31
	nodeRefBytesSize       = 1

	// nodeHeaderSize defines the total size of the header part
	nodeHeaderSize = nodeObfuscationKeySize + versionHashSize + nodeRefBytesSize
)

// Node fork constats.
const (
	nodeForkTypeBytesSize    = 1
	nodeForkPrefixBytesSize  = 1
	nodeForkHeaderSize       = nodeForkTypeBytesSize + nodeForkPrefixBytesSize // 2
	nodeForkPreReferenceSize = 32
	nodePrefixMaxSize        = nodeForkPreReferenceSize - nodeForkHeaderSize // 30
	// "mantaray:0.2"
	nodeForkMetadataBytesSize = 2
)

var (
	version01HashBytes []byte
	version02HashBytes []byte
	zero32             []byte
)

func init() {
	initVersion(version01HashString, &version01HashBytes)
	initVersion(version02HashString, &version02HashBytes)
	zero32 = make([]byte, 32)
}

func initVersion(hash string, bytes *[]byte) {
	b, err := hex.DecodeString(hash)
	if err != nil {
		panic(err)
	}

	*bytes = make([]byte, versionHashSize)
	copy(*bytes, b)
}

var (
	// ErrTooShort signals too short input.
	ErrTooShort = errors.New("serialised input too short")
	// ErrInvalidInput signals invalid input to serialise.
	ErrInvalidInput = errors.New("input invalid")
	// ErrInvalidVersionHash signals unknown version of hash.
	ErrInvalidVersionHash = errors.New("invalid version hash")
)

var obfuscationKeyFn = rand.Read

// SetObfuscationKeyFn allows configuring custom function for generating
// obfuscation key.
//
// NOTE: This should only be used in tests.
func SetObfuscationKeyFn(fn func([]byte) (int, error)) {
	obfuscationKeyFn = fn
}

// MarshalBinary serialises the node
func (n *Node) MarshalBinary() (bytes []byte, err error) {
	if n.forks == nil {
		return nil, ErrInvalidInput
	}

	// header

	headerBytes := make([]byte, nodeHeaderSize)

	if len(n.obfuscationKey) == 0 {
		// generate obfuscation key
		obfuscationKey := make([]byte, nodeObfuscationKeySize)
		for i := 0; i < nodeObfuscationKeySize; {
			read, _ := obfuscationKeyFn(obfuscationKey[i:])
			i += read
		}
		n.obfuscationKey = obfuscationKey
	}
	copy(headerBytes[0:nodeObfuscationKeySize], n.obfuscationKey)

	copy(headerBytes[nodeObfuscationKeySize:nodeObfuscationKeySize+versionHashSize], version02HashBytes)

	headerBytes[nodeObfuscationKeySize+versionHashSize] = uint8(n.refBytesSize)

	bytes = append(bytes, headerBytes...)

	// entry

	entryBytes := make([]byte, n.refBytesSize)
	copy(entryBytes, n.entry)
	bytes = append(bytes, entryBytes...)

	// index

	indexBytes := make([]byte, 32)

	var index = &bitsForBytes{}
	for k := range n.forks {
		index.set(k)
	}
	copy(indexBytes, index.bytes())

	bytes = append(bytes, indexBytes...)

	err = index.iter(func(b byte) error {
		f := n.forks[b]
		ref, err := f.bytes()
		if err != nil {
			return fmt.Errorf("%w on byte '%x'", err, []byte{b})
		}
		bytes = append(bytes, ref...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// perform XOR encryption on bytes after obfuscation key
	xorEncryptedBytes := make([]byte, len(bytes))

	copy(xorEncryptedBytes, bytes[0:nodeObfuscationKeySize])

	for i := nodeObfuscationKeySize; i < len(bytes); i += nodeObfuscationKeySize {
		end := i + nodeObfuscationKeySize
		if end > len(bytes) {
			end = len(bytes)
		}

		encrypted := encryptDecrypt(bytes[i:end], n.obfuscationKey)
		copy(xorEncryptedBytes[i:end], encrypted)
	}

	return xorEncryptedBytes, nil
}

// bitsForBytes is a set of bytes represented as a 256-length bitvector
type bitsForBytes struct {
	bits [32]byte
}

func (bb *bitsForBytes) bytes() (b []byte) {
	b = append(b, bb.bits[:]...)
	return b
}

func (bb *bitsForBytes) fromBytes(b []byte) {
	copy(bb.bits[:], b)
}

func (bb *bitsForBytes) set(b byte) {
	bb.bits[b/8] |= 1 << (b % 8)
}

//nolint,unused
func (bb *bitsForBytes) get(b byte) bool { // skipcq: SCC-U1000
	return bb.getUint8(b)
}

func (bb *bitsForBytes) getUint8(i uint8) bool {
	return (bb.bits[i/8]>>(i%8))&1 > 0
}

func (bb *bitsForBytes) iter(f func(byte) error) error {
	for i := uint8(0); ; i++ {
		if bb.getUint8(i) {
			if err := f(i); err != nil {
				return err
			}
		}
		if i == 255 {
			return nil
		}
	}
}

// UnmarshalBinary deserialises a node
func (n *Node) UnmarshalBinary(data []byte) error {
	if len(data) < nodeHeaderSize {
		return ErrTooShort
	}

	n.obfuscationKey = append([]byte{}, data[0:nodeObfuscationKeySize]...)

	// perform XOR decryption on bytes after obfuscation key
	xorDecryptedBytes := make([]byte, len(data))

	copy(xorDecryptedBytes, data[0:nodeObfuscationKeySize])

	for i := nodeObfuscationKeySize; i < len(data); i += nodeObfuscationKeySize {
		end := i + nodeObfuscationKeySize
		if end > len(data) {
			end = len(data)
		}

		decrypted := encryptDecrypt(data[i:end], n.obfuscationKey)
		copy(xorDecryptedBytes[i:end], decrypted)
	}

	data = xorDecryptedBytes

	// Verify version hash.
	versionHash := data[nodeObfuscationKeySize : nodeObfuscationKeySize+versionHashSize]

	if bytes.Equal(versionHash, version01HashBytes) {

		refBytesSize := int(data[nodeHeaderSize-1])

		n.entry = append([]byte{}, data[nodeHeaderSize:nodeHeaderSize+refBytesSize]...)
		offset := nodeHeaderSize + refBytesSize // skip entry
		n.forks = make(map[byte]*fork)
		bb := &bitsForBytes{}
		bb.fromBytes(data[offset:])
		offset += 32 // skip forks
		return bb.iter(func(b byte) error {
			f := &fork{}

			if len(data) < offset+nodeForkPreReferenceSize+refBytesSize {
				err := fmt.Errorf("not enough bytes for node fork: %d (%d)", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize))
				return fmt.Errorf("%w on byte '%x'", err, []byte{b})
			}

			err := f.fromBytes(data[offset : offset+nodeForkPreReferenceSize+refBytesSize])
			if err != nil {
				return fmt.Errorf("%w on byte '%x'", err, []byte{b})
			}

			n.forks[b] = f
			offset += nodeForkPreReferenceSize + refBytesSize
			return nil
		})
	} else if bytes.Equal(versionHash, version02HashBytes) {

		refBytesSize := int(data[nodeHeaderSize-1])

		n.entry = append([]byte{}, data[nodeHeaderSize:nodeHeaderSize+refBytesSize]...)
		offset := nodeHeaderSize + refBytesSize // skip entry
		// Currently we don't persist the root nodeType when we marshal the manifest, as a result
		// the root nodeType information is lost on Unmarshal. This causes issues when we want to
		// perform a path 'Walk' on the root. If there is more than 1 fork, the root node type
		// is an edge, so we will deduce this information from index byte array
		if !bytes.Equal(data[offset:offset+32], zero32) && !n.IsEdgeType() {
			n.makeEdge()
		}
		n.forks = make(map[byte]*fork)
		bb := &bitsForBytes{}
		bb.fromBytes(data[offset:])
		offset += 32 // skip forks
		return bb.iter(func(b byte) error {
			f := &fork{}

			if len(data) < offset+nodeForkTypeBytesSize {
				return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkTypeBytesSize), []byte{b})
			}

			nodeType := data[offset]

			nodeForkSize := nodeForkPreReferenceSize + refBytesSize

			if nodeTypeIsWithMetadataType(nodeType) {
				if len(data) < offset+nodeForkPreReferenceSize+refBytesSize+nodeForkMetadataBytesSize {
					return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize + nodeForkMetadataBytesSize), []byte{b})
				}

				metadataBytesSize := binary.BigEndian.Uint16(data[offset+nodeForkSize : offset+nodeForkSize+nodeForkMetadataBytesSize])

				nodeForkSize += nodeForkMetadataBytesSize
				nodeForkSize += int(metadataBytesSize)

				err := f.fromBytes02(data[offset:offset+nodeForkSize], refBytesSize, int(metadataBytesSize))
				if err != nil {
					return fmt.Errorf("%w on byte '%x'", err, []byte{b})
				}
			} else {
				if len(data) < offset+nodeForkPreReferenceSize+refBytesSize {
					return fmt.Errorf("not enough bytes for node fork: %d (%d) on byte '%x'", (len(data) - offset), (nodeForkPreReferenceSize + refBytesSize), []byte{b})
				}

				err := f.fromBytes(data[offset : offset+nodeForkSize])
				if err != nil {
					return fmt.Errorf("%w on byte '%x'", err, []byte{b})
				}
			}

			n.forks[b] = f
			offset += nodeForkSize
			return nil
		})
	}

	return fmt.Errorf("%x: %w", versionHash, ErrInvalidVersionHash)
}

func (f *fork) fromBytes(b []byte) error {
	nodeType := b[0]
	prefixLen := int(b[1])

	if prefixLen == 0 || prefixLen > nodePrefixMaxSize {
		return fmt.Errorf("invalid prefix length: %d", prefixLen)
	}

	f.prefix = b[nodeForkHeaderSize : nodeForkHeaderSize+prefixLen]
	f.Node = NewNodeRef(b[nodeForkPreReferenceSize:])
	f.Node.nodeType = nodeType

	return nil
}

func (f *fork) fromBytes02(b []byte, refBytesSize, metadataBytesSize int) error {
	nodeType := b[0]
	prefixLen := int(b[1])

	if prefixLen == 0 || prefixLen > nodePrefixMaxSize {
		return fmt.Errorf("invalid prefix length: %d", prefixLen)
	}

	f.prefix = b[nodeForkHeaderSize : nodeForkHeaderSize+prefixLen]
	f.Node = NewNodeRef(b[nodeForkPreReferenceSize : nodeForkPreReferenceSize+refBytesSize])
	f.Node.nodeType = nodeType

	if metadataBytesSize > 0 {
		metadataBytes := b[nodeForkPreReferenceSize+refBytesSize+nodeForkMetadataBytesSize:]

		metadata := make(map[string]string)
		// using JSON encoding for metadata
		err := json.Unmarshal(metadataBytes, &metadata)
		if err != nil {
			return err
		}

		f.Node.metadata = metadata
	}

	return nil
}

func (f *fork) bytes() (b []byte, err error) {
	r := refBytes(f)
	// using 1 byte ('f.Node.refBytesSize') for size
	if len(r) > 256 {
		err = fmt.Errorf("node reference size > 256: %d", len(r))
		return
	}
	b = append(b, f.Node.nodeType, uint8(len(f.prefix)))

	prefixBytes := make([]byte, nodePrefixMaxSize)
	copy(prefixBytes, f.prefix)
	b = append(b, prefixBytes...)

	refBytes := make([]byte, len(r))
	copy(refBytes, r)
	b = append(b, refBytes...)

	if f.Node.IsWithMetadataType() {
		// using JSON encoding for metadata
		metadataJSONBytes, err1 := json.Marshal(f.Node.metadata)
		if err1 != nil {
			return b, err1
		}

		metadataJSONBytesSizeWithSize := len(metadataJSONBytes) + nodeForkMetadataBytesSize

		// pad JSON bytes if necessary
		if metadataJSONBytesSizeWithSize < nodeObfuscationKeySize {
			paddingLength := nodeObfuscationKeySize - metadataJSONBytesSizeWithSize
			padding := make([]byte, paddingLength)
			for i := range padding {
				padding[i] = '\n'
			}
			metadataJSONBytes = append(metadataJSONBytes, padding...)
		} else if metadataJSONBytesSizeWithSize > nodeObfuscationKeySize {
			paddingLength := nodeObfuscationKeySize - metadataJSONBytesSizeWithSize%nodeObfuscationKeySize
			padding := make([]byte, paddingLength)
			for i := range padding {
				padding[i] = '\n'
			}
			metadataJSONBytes = append(metadataJSONBytes, padding...)
		}

		metadataJSONBytesSize := len(metadataJSONBytes)
		if metadataJSONBytesSize > int(maxUint16) {
			return b, ErrMetadataTooLarge
		}

		mBytesSize := make([]byte, nodeForkMetadataBytesSize)
		binary.BigEndian.PutUint16(mBytesSize, uint16(metadataJSONBytesSize))
		b = append(b, mBytesSize...)

		b = append(b, metadataJSONBytes...)
	}

	return b, nil
}

var refBytes = nodeRefBytes

func nodeRefBytes(f *fork) []byte {
	return f.Node.ref
}

// encryptDecrypt runs a XOR encryption on the input bytes, encrypting it if it
// hasn't already been, and decrypting it if it has, using the key provided.
func encryptDecrypt(input, key []byte) []byte {
	output := make([]byte, len(input))

	for i := 0; i < len(input); i++ {
		output[i] = input[i] ^ key[i%len(key)]
	}

	return output
}
