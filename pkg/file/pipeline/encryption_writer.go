package pipeline

import (
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/swarm"
	"golang.org/x/crypto/sha3"
)

type encryptionWriter struct {
	next chainWriter
}

func newEncryptionWriter(next chainWriter) chainWriter {
	return &encryptionWriter{
		next: next,
	}
}

// Write assumes that the span is prepended to the actual data before the write !
func (e *encryptionWriter) chainWrite(p *pipeWriteArgs) error {
	key, encryptedSpan, encryptedData, err := encrypt(p.data)
	if err != nil {
		return err
	}
	c := make([]byte, len(encryptedSpan)+len(encryptedData))
	copy(c[:8], encryptedSpan)
	copy(c[8:], encryptedData)
	p.data = c // replace the verbatim data with the encrypted data
	p.key = key
	return e.next.chainWrite(p)
}

func (e *encryptionWriter) sum() ([]byte, error) {
	return e.next.sum()
}

func encrypt(chunkData []byte) (encryption.Key, []byte, []byte, error) {
	key := encryption.GenerateRandomKey(encryption.KeyLength)
	encryptedSpan, err := newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, nil, err
	}
	encryptedData, err := newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, nil, err
	}
	return key, encryptedSpan, encryptedData, nil
}

func newSpanEncryption(key encryption.Key) encryption.Interface {
	refSize := int64(swarm.HashSize + encryption.KeyLength)
	return encryption.New(key, 0, uint32(swarm.ChunkSize/refSize), sha3.NewLegacyKeccak256)
}

func newDataEncryption(key encryption.Key) encryption.Interface {
	return encryption.New(key, int(swarm.ChunkSize), 0, sha3.NewLegacyKeccak256)
}
