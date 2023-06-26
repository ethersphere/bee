package storage

import lru "github.com/hashicorp/golang-lru/v2"

type cache struct {
	s Store
	c *lru.Cache[string, []byte]
}

func NewCacheLayer(store Store, maxCapacity int) Store {
	c, _ := lru.New[string, []byte](maxCapacity)
	return &cache{store, c}
}

func (c *cache) Get(i Item) error {
	if val, ok := c.c.Get(i.ID()); ok {
		return i.Unmarshal(val)
	}
	err := c.s.Get(i)
	if err != nil {
		return err
	}

	c.addCache(i)

	return nil
}

func (c *cache) Has(k Key) (bool, error) {
	if _, ok := c.c.Get(k.ID()); ok {
		return true, nil
	}
	return c.s.Has(k)
}

func (c *cache) GetSize(k Key) (int, error) {
	return c.s.GetSize(k)
}

func (c *cache) Iterate(q Query, f IterateFn) error {
	return c.s.Iterate(q, f)
}

func (c *cache) Count(k Key) (int, error) {
	return c.s.Count(k)
}

func (c *cache) Put(i Item) error {
	c.addCache(i)
	return c.s.Put(i)
}

func (c *cache) Delete(i Item) error {
	_ = c.c.Remove(i.ID())
	return c.s.Delete(i)
}

func (c *cache) Close() error {
	return c.s.Close()
}

func (c *cache) addCache(i Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.c.Add(i.ID(), b)
}
