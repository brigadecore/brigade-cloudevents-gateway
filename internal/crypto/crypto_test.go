package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHash(t *testing.T) {
	testCases := []struct {
		name string
		salt string
	}{
		{
			name: "without salt",
			salt: "",
		},
		{
			name: "with salt",
			salt: "n pepa",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			const testInput = "shoop"
			hash := Hash(testCase.salt, testInput)
			assert.NotEqual(t, testInput, hash)
			// This is how long a sha256 sum should be
			assert.Equal(t, 64, len(hash))
		})
	}
}
