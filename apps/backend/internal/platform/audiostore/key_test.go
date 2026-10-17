package audiostore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewKey_IsValidUniqueAndRevealsNothing(t *testing.T) {
	k1, k2 := NewKey(), NewKey()
	assert.True(t, validKey(k1))
	assert.NotEqual(t, k1, k2)
}
