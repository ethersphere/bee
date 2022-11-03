package bzz

func AddressSliceContains(addrs []Address, a *Address) bool {
	for _, v := range addrs {
		if v.Equal(a) {
			return true
		}
	}
	return false
}
