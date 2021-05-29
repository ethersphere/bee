package file_test

import (
	"fmt"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/keystore/file"
	"testing"
)

func TestKey(t *testing.T) {
	str := ""
	var data []byte = []byte(str)
	password := ""
	pk, err := file.DecryptKey(data, password)
	if err != nil {
		print(err)
	}
	p := crypto.EncodeSecp256k1PrivateKey(pk)
	fmt.Printf("0x%x", p)
}
