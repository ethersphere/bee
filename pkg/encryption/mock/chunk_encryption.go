package mock

import (
	"github.com/ethersphere/bee/pkg/encryption"
)

type ce struct {
	key []byte
}

func NewChunkEncrypter(key []byte) encryption.ChunkEncrypter { return &ce{key: key} }

func (c *ce) EncryptChunk(chunkData []byte) (encryption.Key, []byte, []byte, error) {
	enc := New(WithXOREncryption(c.key))
	encryptedSpan, err := enc.Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, nil, err
	}
	encryptedData, err := enc.Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, nil, err
	}
	return nil, encryptedSpan, encryptedData, nil
}
