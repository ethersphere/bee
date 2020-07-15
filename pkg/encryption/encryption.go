// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package encryption

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"hash"
	"sync"
)

const KeyLength = 32

type Key []byte

type Encryptor interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
	Reset()
}

type Encryption struct {
	key      Key              // the encryption key (hashSize bytes long)
	keyLen   int              // length of the key = length of blockcipher block
	padding  int              // encryption will pad the data upto this if > 0
	index    int              // counter index
	initCtr  uint32           // initial counter used for counter mode blockcipher
	hashFunc func() hash.Hash // hasher constructor function
}

// New constructs a new encryptor/decryptor
func New(key Key, padding int, initCtr uint32, hashFunc func() hash.Hash) *Encryption {
	return &Encryption{
		key:      key,
		keyLen:   len(key),
		padding:  padding,
		initCtr:  initCtr,
		hashFunc: hashFunc,
	}
}

// Encrypt encrypts the data and does padding if specified
func (e *Encryption) Encrypt(data []byte) ([]byte, error) {
	length := len(data)
	outLength := length
	isFixedPadding := e.padding > 0
	if isFixedPadding {
		if length > e.padding {
			return nil, fmt.Errorf("data length longer than padding, data length %v padding %v", length, e.padding)
		}
		outLength = e.padding
	}
	out := make([]byte, outLength)
	err := e.transform(data, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Decrypt decrypts the data, if padding was used caller must know original length and truncate
func (e *Encryption) Decrypt(data []byte) ([]byte, error) {
	length := len(data)
	if e.padding > 0 && length != e.padding {
		return nil, fmt.Errorf("data length different than padding, data length %v padding %v", length, e.padding)
	}
	out := make([]byte, length)
	err := e.transform(data, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Reset resets the counter. It is only safe to call after an encryption operation is completed
// After Reset is called, the Encryption object can be re-used for other data
func (e *Encryption) Reset() {
	e.index = 0
}

// split up input into keylength segments and encrypt sequentially
func (e *Encryption) transform(in, out []byte) error {
	inLength := len(in)
	wg := sync.WaitGroup{}
	wg.Add((inLength-1)/e.keyLen + 1)

	for i := 0; i < inLength; i += e.keyLen {
		errs := make(chan error, 1)
		l := min(e.keyLen, inLength-i)
		go func(i int, x, y []byte) {
			defer wg.Done()
			err := e.Transcrypt(i, x, y)
			errs <- err
		}(e.index, in[i:i+l], out[i:i+l])
		e.index++
		err := <-errs
		if err != nil {
			close((errs))
			return err
		}
	}
	// pad the rest if out is longer
	pad(out[inLength:])
	wg.Wait()
	return nil
}

// used for segmentwise transformation
// if in is shorter than out, padding is used
func (e *Encryption) Transcrypt(i int, in, out []byte) error {
	// first hash key with counter (initial counter + i)
	hasher := e.hashFunc()
	_, err := hasher.Write(e.key)
	if err != nil {
		return err
	}

	ctrBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(ctrBytes, uint32(i)+e.initCtr)
	_, err = hasher.Write(ctrBytes)
	if err != nil {
		return err
	}
	ctrHash := hasher.Sum(nil)
	hasher.Reset()

	// second round of hashing for selective disclosure
	_, err = hasher.Write(ctrHash)
	if err != nil {
		return err
	}
	segmentKey := hasher.Sum(nil)
	hasher.Reset()

	// XOR bytes uptil length of in (out must be at least as long)
	inLength := len(in)
	for j := 0; j < inLength; j++ {
		out[j] = in[j] ^ segmentKey[j]
	}
	// insert padding if out is longer
	pad(out[inLength:])

	return nil
}

func pad(b []byte) {
	l := len(b)
	for total := 0; total < l; {
		read, _ := rand.Read(b[total:])
		total += read
	}
}

// GenerateRandomKey generates a random key of length l
func GenerateRandomKey(l int) Key {
	key := make([]byte, l)
	var total int
	for total < l {
		read, _ := rand.Read(key[total:])
		total += read
	}
	return key
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
