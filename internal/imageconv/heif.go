package imageconv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func NeedsHEIFConversion(extension string) bool {
	return extension == "heic" || extension == "heif"
}

func HEIFToJPEG(ctx context.Context, data []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "mealie-importer-heif-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input.heic")
	output := filepath.Join(dir, "output.jpg")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "heif-dec", "--quiet", "--quality", "95", "--output", output, input)
	if combined, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("convert HEIF image to JPEG: %w: %s", err, combined)
	}

	converted, err := os.ReadFile(output)
	if err != nil {
		return nil, err
	}
	if len(converted) == 0 {
		return nil, fmt.Errorf("convert HEIF image to JPEG: converter produced an empty file")
	}

	return converted, nil
}
