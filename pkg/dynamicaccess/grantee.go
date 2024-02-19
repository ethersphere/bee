package dynamicaccess

type Grantee interface {
	Revoke(topic string) error
	RevokeList(topic string, removeList []string, addList []string) (string, error)
	Publish(topic string) error
}

type defaultGrantee struct {
}

func (g *defaultGrantee) Revoke(topic string) error {
	return nil
}

func (g *defaultGrantee) RevokeList(topic string, removeList []string, addList []string) (string, error) {
	return "", nil
}

func (g *defaultGrantee) Publish(topic string) error {
	return nil
}

func NewGrantee() Grantee {
	return &defaultGrantee{}
}
