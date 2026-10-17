package audiostore_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/audiostore/storetest"
)

func TestLocalFS_Contract(t *testing.T) {
	s, err := audiostore.NewLocalFS(filepath.Join(t.TempDir(), "audio"))
	require.NoError(t, err)
	storetest.Run(t, s)
}

func TestMemory_Contract(t *testing.T) {
	storetest.Run(t, audiostore.NewMemory())
}

func TestLocalFS_ShortWriteLeavesNoFileBehind(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audio")
	s, err := audiostore.NewLocalFS(dir)
	require.NoError(t, err)

	require.Error(t, s.Put(context.Background(), audiostore.NewKey(), strings.NewReader("short"), 100))

	entries, _ := os.ReadDir(dir)
	assert.Empty(t, entries, "no partial object and no temp file may remain")
}
