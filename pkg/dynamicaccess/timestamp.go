package dynamicaccess

// container interface bee-ből a manifest
type Timestamp interface{}

type defaultTimeStamp struct{}

func NewTimestamp() Timestamp {
	return &defaultTimeStamp{}
}
