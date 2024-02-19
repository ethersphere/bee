package dynamicaccess

type Act interface{}

type defaultAct struct {
}

func (a *defaultAct) Add(oldItemKey string, oldRootHash string) (newRootHash string, err error) {
	return "", nil
}

func (a *defaultAct) Get(rootKey string) (value string, err error) {
	return "", nil
}

func NewAct() Container {
	return &defaultAct{}
}
