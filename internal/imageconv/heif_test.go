package imageconv

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNeedsHEIFConversion(t *testing.T) {
	assert.True(t, NeedsHEIFConversion("heic"))
	assert.True(t, NeedsHEIFConversion("heif"))
	assert.False(t, NeedsHEIFConversion("jpg"))
}

func TestHEIFToJPEGUsesHeifDec(t *testing.T) {
	dir := t.TempDir()
	converter := filepath.Join(dir, "heif-dec")
	script := `#!/bin/sh
set -eu
out=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --output)
      out="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
printf '\377\330\377converted' > "$out"
`
	require.NoError(t, os.WriteFile(converter, []byte(script), 0o700))
	t.Setenv("PATH", dir)

	got, err := HEIFToJPEG(context.Background(), []byte("heif data"))
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xd8, 0xff, 'c', 'o', 'n', 'v', 'e', 'r', 't', 'e', 'd'}, got)
}
