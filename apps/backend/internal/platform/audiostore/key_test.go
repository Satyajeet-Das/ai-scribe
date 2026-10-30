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

func TestKinds_DoNotAcceptEachOthersKeys(t *testing.T) {
	wav, pdf := NewKeyFor(KindWAV), NewKeyFor(KindPDF)
	assert.True(t, validKeyFor(KindWAV, wav))
	assert.True(t, validKeyFor(KindPDF, pdf))
	assert.False(t, validKeyFor(KindWAV, pdf), "an audio store must not see exam documents")
	assert.False(t, validKeyFor(KindPDF, wav))
}
