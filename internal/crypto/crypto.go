package crypto

import (
	"crypto/sha256"
	"fmt"
)

// Hash returns a secure hash of the provided input. If a salt is provided, the
// input is prepended with the salt prior to hashing.
func Hash(salt, input string) string {
	if salt != "" {
		input = fmt.Sprintf("%s:%s", salt, input)
	}
	sum := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", sum)
}
