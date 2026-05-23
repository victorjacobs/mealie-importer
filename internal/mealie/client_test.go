package mealie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCreatesAndUpdatesRecipe(t *testing.T) {
	var created CreateRecipe
	var updated Recipe

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/recipes":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&created))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"test-recipe"`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/recipes/test-recipe":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&updated))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "test-token")
	require.NoError(t, err)

	slug, err := client.CreateRecipe(context.Background(), "Test Recipe")
	require.NoError(t, err)
	require.Equal(t, "test-recipe", slug)

	err = client.UpdateRecipe(context.Background(), slug, Recipe{Name: "Test Recipe"})
	require.NoError(t, err)

	assert.Equal(t, "Test Recipe", created.Name)
	assert.Equal(t, "Test Recipe", updated.Name)
}

func TestClientFindsRecipeByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/recipes", r.URL.Path)
		assert.Equal(t, "Test Recipe", r.URL.Query().Get("search"))
		assert.Equal(t, "50", r.URL.Query().Get("perPage"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{
			"items": [
				{"name": "Other Recipe", "slug": "other-recipe"},
				{"name": "test recipe", "slug": "test-recipe"}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "test-token")
	require.NoError(t, err)

	got, ok, err := client.FindRecipeByName(context.Background(), "Test Recipe")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "test-recipe", got.Slug)
}

func TestClientDoesNotFindPartialRecipeName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"items": [
				{"name": "Test Recipe Deluxe", "slug": "test-recipe-deluxe"}
			]
		}`))
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "test-token")
	require.NoError(t, err)

	_, ok, err := client.FindRecipeByName(context.Background(), "Test Recipe")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestClientUploadsRecipeImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/recipes/test-recipe/image", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		reader, err := r.MultipartReader()
		require.NoError(t, err)

		parts := map[string]string{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			data, err := io.ReadAll(part)
			require.NoError(t, err)
			parts[part.FormName()] = string(data)
			if part.FormName() == "image" {
				assert.Equal(t, "image.jpg", part.FileName())
			}
		}

		assert.Equal(t, "jpg", parts["extension"])
		assert.Equal(t, "\xff\xd8\xff", parts["image"])
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "test-token")
	require.NoError(t, err)

	err = client.UploadRecipeImage(context.Background(), "test-recipe", []byte{0xff, 0xd8, 0xff}, "jpg")
	require.NoError(t, err)
}

func TestNewClientValidatesConfig(t *testing.T) {
	_, err := NewClient("", "token")
	assert.ErrorContains(t, err, "base URL is required")

	_, err = NewClient("https://example.com", "")
	assert.ErrorContains(t, err, "token is required")
}
