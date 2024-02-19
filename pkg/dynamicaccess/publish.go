package dynamicaccess

type Publish interface {
	upload(ref string) (string, error)
}

type DefaultPublish struct {
}

func (d *DefaultPublish) upload(ref string) (string, error) {
	return "default", nil
}
