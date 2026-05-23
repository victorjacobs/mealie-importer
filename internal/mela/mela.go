package mela

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	Data      []byte
	MediaType string
	Extension string
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
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".melarecipe" {
			continue
		}
		paths = append(paths, filepath.Join(path, entry.Name()))
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
