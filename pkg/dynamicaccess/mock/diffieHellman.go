package mock

type DiffieHellmanMock struct {
	SharedSecretFunc func(string, string, []byte) (string, error)
}

func (ma *DiffieHellmanMock) SharedSecret(publicKey string, tag string, moment []byte) (string, error) {
	if ma.SharedSecretFunc == nil {
		return "", nil
	}
	return ma.SharedSecretFunc(publicKey, tag, moment)
}
