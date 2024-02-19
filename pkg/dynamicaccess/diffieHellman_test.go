package dynamicaccess

import "testing"

func TestSharedSecret(t *testing.T) {
	_, err := NewDiffieHellman(nil).SharedSecret("", "", nil)
	if err != nil {
		t.Errorf("Error generating shared secret: %v", err)
	}
}
