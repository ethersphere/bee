package mock

type ContainerMock struct {
	AddFunc func(string, string, string) error
	GetFunc func(string, string, string) (string, error)
}

func (ma *ContainerMock) Add(ref string, publisher string, tag string) error {
	if ma.AddFunc == nil {
		return nil
	}
	return ma.AddFunc(ref, publisher, tag)
}

func (ma *ContainerMock) Get(ref string, publisher string, tag string) (string, error) {
	if ma.GetFunc == nil {
		return "", nil
	}
	return ma.GetFunc(ref, publisher, tag)
}
