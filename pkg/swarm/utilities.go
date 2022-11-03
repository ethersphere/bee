package swarm

func AddressSliceContains(addrs []Address, a Address) bool {
	for _, v := range addrs {
		if a.Equal(v) {
			return true
		}
	}
	return false
}
