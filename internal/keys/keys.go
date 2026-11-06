// Package keys generates API keys.
package keys

import (
	"crypto/rand"
	"crypto/subtle"
	"math/big"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// New returns "ezlm-" followed by 40 base62 characters (~238 bits).
func New() string {
	b := make([]byte, 40)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		b[i] = alphabet[n.Int64()]
	}
	return "ezlm-" + string(b)
}

// Equal reports whether two keys are the same, in time that does not depend
// on where they first differ. An empty key never matches, not even another
// empty one: subtle.ConstantTimeCompare("", "") is 1, and an empty expected
// key must not turn into an open door.
func Equal(a, b string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
