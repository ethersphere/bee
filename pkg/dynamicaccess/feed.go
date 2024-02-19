package dynamicaccess

// referencia: history.go
type Feed interface {
	Update(itemKey string, content string) error
	Get(itemKey string) (content string, err error)
	AddNewGrantee(itemKey string, grantee string) error
	RemoveGrantee(itemKey string, grantee string) error
	GetAccess(encryptedRef string, publisher string, tag string) (access string, err error)
}

type defaultFeed struct{}

func (f *defaultFeed) Update(itemKey string, content string) error {
	return nil
}

func (f *defaultFeed) Get(itemKey string) (content string, err error) {
	return "", nil
}

func (f *defaultFeed) AddNewGrantee(itemKey string, grantee string) error {
	return nil
}

func (f *defaultFeed) RemoveGrantee(itemKey string, grantee string) error {
	return nil
}

func (f *defaultFeed) GetAccess(encryptedRef string, publisher string, tag string) (access string, err error) {
	return "", nil
}

func NewFeed() Feed {
	return &defaultFeed{}
}
