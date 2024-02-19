package dynamicaccess

// TODO FROM BEE!!!!
// timestamp alapú history
type History interface {
	Add(oldItemKey string, oldRootHash string) (newRootHash string, err error)
	Get(rootKey string) (value string, err error)
}

type defaultHistory struct{}

func (h *defaultHistory) Add(oldItemKey string, oldRootHash string) (newRootHash string, err error) {
	return "", nil
}

func (h *defaultHistory) Get(rootKey string) (value string, err error) {
	return "", nil
}

func NewHistory() History {
	return &defaultHistory{}
}
