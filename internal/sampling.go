package internal

import "math/rand"

// ShouldSample calculates whether or not to sample given the provided ratio.
// Ratio here means a 1 in X change of being selected.
func ShouldSample(ratio int) bool {
	if ratio <= 0 {
		return false
	} else if ratio == 1 {
		return true
	}

	return rand.Float64() < 1/float64(ratio) //nolint:gosec // doesn't need cryptographic security
}
