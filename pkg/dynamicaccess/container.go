package dynamicaccess

// iterator
type Container interface {
	Add(oldItemKey string, oldRootHash string) (newRootHash string, err error)
	Get(rootKey string) (value string, err error)
}
