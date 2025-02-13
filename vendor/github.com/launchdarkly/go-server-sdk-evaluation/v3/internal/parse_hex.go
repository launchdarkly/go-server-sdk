package internal

// ParseHexUint64 is equivalent to using strconv.ParseUint with base 16, except that it operates directly
// on a byte slice without having to allocate a string. (A version of strconv.ParseUint that operates on
// a byte slice has been a frequently requested Go feature for years, but as of the time this comment
// was written, those requests have not been acted on.)
func ParseHexUint64(data []byte) (uint64, bool) {
	if len(data) == 0 {
		return 0, false
	}
	var ret uint64
	for _, ch := range data {
		ret <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			ret += uint64(ch - '0')
		case ch >= 'a' && ch <= 'f':
			ret += uint64(ch - 'a' + 10)
		case ch >= 'A' && ch <= 'F':
			ret += uint64(ch - 'A' + 10)
		default:
			return 0, false
		}
	}
	return ret, true
}
