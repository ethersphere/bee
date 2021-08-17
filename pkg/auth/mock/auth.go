package mock

type Auth struct {
	AuthorizeFunc func(string, string) bool
	AddKeyFunc    func(string) (string, error)
}

func (ma *Auth) Authorize(u string, p string) bool {
	if ma.AuthorizeFunc == nil {
		return true
	}
	return ma.AuthorizeFunc(u, p)
}
func (ma *Auth) AddKey(k string) (string, error) {
	if ma.AddKeyFunc == nil {
		return "", nil
	}
	return ma.AddKeyFunc(k)
}
func (ma *Auth) Enforce(string, string, string) (bool, error) {
	return false, nil
}
