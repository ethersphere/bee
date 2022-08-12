package storage

import "context"

type Serializable interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

// Simple item which can be used in the Store
type Item interface {
	Serializable
	// Namespace is used to separate similar items.
	// Can be seen as a table construct.
	Namespace() string
	// ID is the unique identifier for this Item.
	// This can be constructed in the function call from the item.
	ID() string
}

// Store contains the interfaces required for the Data Abstraction Layer
type Store interface {
	Get(Item) error
	Has(Item) (bool, error)
	GetSize(Item) (int, error)
	Iterate(Query, IterateFn)
	Count(Item) (int, error)
	Put(Item) error
	Delete(Item) error
}

type Result struct {
	Key string
	// Entry in the result. This can be an Item either constructed from reading
	// the DB or it can be just with the Key and size if we only want size query.
	Entry Item
	// Contains size if it is size query
	Size int
}

type IterateFn func(Result) (bool, error)

// Filter can be used to subtract entries from the iteration. This would enable
// IterateFns to be simpler and not worry about other entries.
type Filter func(string, []byte) bool

type Order int

const (
	AscendingOrder  Order = 0
	DescendingOrder Order = 1
)

// Query parameters for iterating. The Namespace would be the common prefix of the
// keys that needs to be iterated.
type Query struct {
	// Constructor passed by client to construct new object for result
	Factory func() Item
	// Used when we dont want to unmarshal the data from the DB
	KeysOnly bool
	// Used in case we only care about size of entry
	SizeOnly bool
	// Order of operation
	Order Order
	// optional
	Filters []Filter
}

type Txn interface {
	Store

	Commit(context.Context) error
	Rollback(context.Context) error
}
