package mock

type AccessLogicMock struct {
	GetFunc func(string, string, string) (string, error)
}

func (ma *AccessLogicMock) Get(encryped_ref string, publisher string, tag string) (string, error) {
	if ma.GetFunc == nil {
		return "", nil
	}
	return ma.GetFunc(encryped_ref, publisher, tag)
}
