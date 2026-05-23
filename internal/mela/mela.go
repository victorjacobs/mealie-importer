package mela

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const cfAbsoluteTimeUnixOffset = 978307200

type Recipe struct {
	Path         string
	PrepTime     string   `json:"prepTime"`
	Categories   []string `json:"categories"`
	CookTime     string   `json:"cookTime"`
	TotalTime    string   `json:"totalTime"`
	Ingredients  string   `json:"ingredients"`
	Text         string   `json:"text"`
	Yield        string   `json:"yield"`
	Link         string   `json:"link"`
	Favorite     bool     `json:"favorite"`
	WantToCook   bool     `json:"wantToCook"`
	Title        string   `json:"title"`
	Notes        string   `json:"notes"`
	ID           string   `json:"id"`
	Date         float64  `json:"date"`
	Instructions string   `json:"instructions"`
	Nutrition    string   `json:"nutrition"`
	Images       []string `json:"images"`
}

type Image struct {
	Data          []byte
	MediaType     string
	Extension     string
	ConvertedFrom string
}

func ReadSource(path string) ([]Recipe, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		recipes, err := ReadDir(path)
		return recipes, func() {}, err
	}
	if filepath.Ext(path) != ".melarecipes" {
		return nil, nil, fmt.Errorf("%s: expected a directory or .melarecipes zip file", path)
	}

	dir, err := os.MkdirTemp("", "mealie-importer-mela-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}

	if err := unzip(path, dir); err != nil {
		cleanup()
		return nil, nil, err
	}

	recipes, err := ReadDir(dir)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return recipes, cleanup, nil
}

func ReadFile(path string) (Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}

	var recipe Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return Recipe{}, fmt.Errorf("decode %s: %w", path, err)
	}
	recipe.Path = path
	if strings.TrimSpace(recipe.Title) == "" {
		return Recipe{}, fmt.Errorf("%s: missing title", path)
	}

	return recipe, nil
}

func ReadDir(path string) ([]Recipe, error) {
	var paths []string
	err := filepath.WalkDir(path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".melarecipe" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	recipes := make([]Recipe, 0, len(paths))
	for _, path := range paths {
		recipe, err := ReadFile(path)
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

func unzip(source, targetDir string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target, err := safeZipPath(targetDir, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := extractZipFile(file, target); err != nil {
			return err
		}
	}
	return nil
}

func safeZipPath(targetDir, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if cleanName == "." || filepath.IsAbs(cleanName) || strings.HasPrefix(cleanName, ".."+string(os.PathSeparator)) || cleanName == ".." {
		return "", fmt.Errorf("unsafe zip path %q", name)
	}

	target := filepath.Join(targetDir, cleanName)
	cleanTargetDir := filepath.Clean(targetDir) + string(os.PathSeparator)
	cleanTarget := filepath.Clean(target)
	if !strings.HasPrefix(cleanTarget+string(os.PathSeparator), cleanTargetDir) && cleanTarget != filepath.Clean(targetDir) {
		return "", fmt.Errorf("unsafe zip path %q", name)
	}
	return target, nil
}

func extractZipFile(file *zip.File, target string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func (r Recipe) DateAdded() string {
	if r.Date <= 0 {
		return ""
	}
	unixSeconds := int64(r.Date) + cfAbsoluteTimeUnixOffset
	return time.Unix(unixSeconds, 0).UTC().Format(time.DateOnly)
}

func (r Recipe) IngredientLines() []string {
	return nonEmptyLines(r.Ingredients)
}

func (r Recipe) InstructionSteps() []string {
	return nonEmptyLines(r.Instructions)
}

func (r Recipe) PrimaryImage() (Image, bool, error) {
	if len(r.Images) == 0 || strings.TrimSpace(r.Images[0]) == "" {
		return Image{}, false, nil
	}

	data, err := base64.StdEncoding.DecodeString(r.Images[0])
	if err != nil {
		return Image{}, false, fmt.Errorf("%s: decode primary image: %w", r.Path, err)
	}

	mediaType, extension := detectImage(data)
	return Image{
		Data:      data,
		MediaType: mediaType,
		Extension: extension,
	}, true, nil
}

func nonEmptyLines(input string) []string {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func detectImage(data []byte) (string, string) {
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", "jpg"
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", "png"
	}
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return "image/gif", "gif"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		brand := string(data[8:12])
		switch brand {
		case "heic", "heix", "hevc", "hevx", "mif1", "msf1":
			return "image/heic", "heic"
		case "heif":
			return "image/heif", "heif"
		}
	}
	return "application/octet-stream", "bin"
}
