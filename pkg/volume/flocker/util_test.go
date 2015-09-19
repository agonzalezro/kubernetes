package flocker

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetenvOrFallback(t *testing.T) {
	const expected = "foo"

	assert := assert.New(t)

	key := "FLOCKER_SET_VAR"
	os.Setenv(key, expected)
	assert.Equal(GetenvOrFallback(key, "~"+expected), expected)

	key = "FLOCKER_UNSET_VAR"
	assert.Equal(GetenvOrFallback(key, expected), expected)
}
