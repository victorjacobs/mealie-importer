package mealie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return e.Status
	}
	return fmt.Sprintf("%s: %s", e.Status, e.Body)
}

func IsAlreadyExists(err error) bool {
	var httpErr *HTTPError
	return errors.As(err, &httpErr) &&
		httpErr.StatusCode == http.StatusBadRequest &&
		strings.Contains(httpErr.Body, "Recipe already exists")
}

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base URL must include scheme and host")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("token is required")
	}

	return &Client{
		baseURL: parsed,
		token:   token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (c *Client) CreateRecipe(ctx context.Context, name string) (string, error) {
	var slug string
	if err := c.doJSON(ctx, http.MethodPost, "/api/recipes", CreateRecipe{Name: name}, &slug); err != nil {
		return "", err
	}
	return slug, nil
}

func (c *Client) FindRecipeByName(ctx context.Context, name string) (RecipeSummary, bool, error) {
	endpoint := c.endpoint("/api/recipes")
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return RecipeSummary{}, false, err
	}
	query := parsed.Query()
	query.Set("search", name)
	query.Set("perPage", "50")
	parsed.RawQuery = query.Encode()

	var results RecipeSearchResults
	if err := c.doJSONURL(ctx, http.MethodGet, parsed.String(), nil, &results); err != nil {
		return RecipeSummary{}, false, err
	}

	for _, item := range results.Items {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(name)) {
			return item, true, nil
		}
	}
	return RecipeSummary{}, false, nil
}

func (c *Client) FindRecipeBySlug(ctx context.Context, slug string) (RecipeSummary, bool, error) {
	recipe, found, err := c.GetRecipe(ctx, slug)
	if err != nil || !found {
		return RecipeSummary{}, false, err
	}
	return RecipeSummary{Name: recipe.Name, Slug: recipe.Slug}, true, nil
}

func (c *Client) GetRecipe(ctx context.Context, slug string) (Recipe, bool, error) {
	var recipe Recipe
	err := c.doJSON(ctx, http.MethodGet, "/api/recipes/"+url.PathEscape(slug), nil, &recipe)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return Recipe{}, false, nil
		}
		return Recipe{}, false, err
	}
	if recipe.Slug == "" {
		recipe.Slug = slug
	}
	return recipe, true, nil
}

func (c *Client) UpdateRecipe(ctx context.Context, slug string, recipe Recipe) error {
	return c.doJSON(ctx, http.MethodPut, "/api/recipes/"+url.PathEscape(slug), recipe, nil)
}

func (c *Client) UploadRecipeImage(ctx context.Context, slug string, image []byte, extension string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "image."+extension)
	if err != nil {
		return err
	}
	if _, err := part.Write(image); err != nil {
		return err
	}
	if err := writer.WriteField("extension", extension); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint("/api/recipes/"+url.PathEscape(slug)+"/image"), &body)
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, input any, output any) error {
	return c.doJSONURL(ctx, method, c.endpoint(endpoint), input, output)
}

func (c *Client) doJSONURL(ctx context.Context, method, url string, input any, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	c.authorize(req)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func (c *Client) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token)
}

func (c *Client) endpoint(endpoint string) string {
	clone := *c.baseURL
	clone.Path = path.Join(c.baseURL.Path, endpoint)
	return clone.String()
}

func responseError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(data))
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       message,
	}
}
