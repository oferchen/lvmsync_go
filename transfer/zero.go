package transfer

// isAllZero returns true if the provided slice consists entirely of zero bytes.
func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
