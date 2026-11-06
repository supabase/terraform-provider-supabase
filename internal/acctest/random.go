package acctest

import (
	"crypto/rand"
	"encoding/hex"
)

const namePrefix = "acc-test"

// RandomName returns a random resource name prefixed with "acc-test",
// e.g. "acc-test-1a2b3c4d5e6f".
func RandomName() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return namePrefix + "-" + hex.EncodeToString(b)
}
