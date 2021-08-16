package auth

type MockAuth struct {
	AuthorizeFunc func(string, string) bool
	AddKeyFunc    func(string) (string, error)
}

func (ma *MockAuth) Authorize(u string, p string) bool {
	return ma.AuthorizeFunc(u, p)
}
func (ma *MockAuth) AddKey(k string) (string, error) {
	return ma.AddKeyFunc(k)
}
func (ma *MockAuth) Enforce(string, string, string) (bool, error) {
	return false, nil
}
