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

	"go.uber.org/zap"
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
	logger     *zap.Logger
}

type Option func(*Client)

func WithLogger(logger *zap.Logger) Option {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
		}
	}
}

func NewClient(baseURL, token string, opts ...Option) (*Client, error) {
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

	client := &Client{
		baseURL: parsed,
		token:   token,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		logger: zap.NewNop(),
	}
	for _, opt := range opts {
		opt(client)
	}
	return client, nil
}

func (c *Client) CreateRecipe(ctx context.Context, name string) (string, error) {
	c.logger.Debug("creating recipe stub", zap.String("name", name))
	var slug string
	if err := c.doJSON(ctx, http.MethodPost, "/api/recipes", CreateRecipe{Name: name}, &slug); err != nil {
		return "", err
	}
	c.logger.Debug("created recipe stub", zap.String("name", name), zap.String("slug", slug))
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

	c.logger.Debug("searching recipes by name", zap.String("name", name), zap.String("url", parsed.String()))
	var results RecipeSearchResults
	if err := c.doJSONURL(ctx, http.MethodGet, parsed.String(), nil, &results); err != nil {
		return RecipeSummary{}, false, err
	}

	for _, item := range results.Items {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(name)) {
			c.logger.Debug("matched recipe by name", zap.String("name", name), zap.String("matchedName", item.Name), zap.String("slug", item.Slug))
			return item, true, nil
		}
	}
	c.logger.Debug("no exact recipe name match", zap.String("name", name), zap.Int("resultCount", len(results.Items)))
	return RecipeSummary{}, false, nil
}

func (c *Client) FindRecipeBySlug(ctx context.Context, slug string) (RecipeSummary, bool, error) {
	c.logger.Debug("searching recipe by slug", zap.String("slug", slug))
	recipe, found, err := c.GetRecipe(ctx, slug)
	if err != nil || !found {
		return RecipeSummary{}, false, err
	}
	return RecipeSummary{Name: recipe.Name, Slug: recipe.Slug}, true, nil
}

func (c *Client) ListCategories(ctx context.Context) ([]Organizer, error) {
	c.logger.Debug("listing categories")
	var categories []Organizer
	for page := 1; ; page++ {
		endpoint := c.endpoint("/api/organizers/categories")
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		query := parsed.Query()
		query.Set("page", fmt.Sprint(page))
		query.Set("perPage", "200")
		parsed.RawQuery = query.Encode()

		var results CategoryResults
		if err := c.doJSONURL(ctx, http.MethodGet, parsed.String(), nil, &results); err != nil {
			return nil, err
		}
		categories = append(categories, results.Items...)
		if results.TotalPages == 0 || page >= results.TotalPages {
			break
		}
	}
	c.logger.Debug("listed categories", zap.Int("count", len(categories)))
	return categories, nil
}

func (c *Client) CreateCategory(ctx context.Context, name string) error {
	c.logger.Debug("creating category", zap.String("name", name))
	if err := c.doJSON(ctx, http.MethodPost, "/api/organizers/categories", CreateCategory{Name: name}, nil); err != nil {
		return err
	}
	c.logger.Debug("created category", zap.String("name", name))
	return nil
}

func (c *Client) GetRecipe(ctx context.Context, slug string) (Recipe, bool, error) {
	c.logger.Debug("getting recipe", zap.String("slug", slug))
	var recipe Recipe

	err := c.doJSON(ctx, http.MethodGet, "/api/recipes/"+url.PathEscape(slug), nil, &recipe)
	if err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			c.logger.Debug("recipe not found", zap.String("slug", slug))
			return Recipe{}, false, nil
		}

		return Recipe{}, false, err
	}

	if recipe.Slug == "" {
		recipe.Slug = slug
	}

	c.logger.Debug("got recipe", zap.String("slug", slug), zap.String("id", recipe.ID), zap.String("name", recipe.Name), zap.String("recipeSlug", recipe.Slug))

	return recipe, true, nil
}

func (c *Client) UpdateRecipe(ctx context.Context, slug string, recipe Recipe) error {
	c.logger.Debug("updating recipe", zap.String("slug", slug), zap.Any("recipe", recipe))

	return c.doJSON(ctx, http.MethodPut, "/api/recipes/"+url.PathEscape(slug), recipe, nil)
}

func (c *Client) UploadRecipeImage(ctx context.Context, slug string, image []byte, extension string) error {
	c.logger.Debug("uploading recipe image", zap.String("slug", slug), zap.String("extension", extension), zap.Int("bytes", len(image)))
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
		c.logger.Debug("mealie image request failed", zap.String("method", req.Method), zap.String("url", req.URL.String()), zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := responseError(resp)
		c.logger.Debug("mealie image request returned error", zap.String("method", req.Method), zap.String("url", req.URL.String()), zap.Int("statusCode", resp.StatusCode), zap.Error(err))
		return err
	}
	c.logger.Debug("mealie image request completed", zap.String("method", req.Method), zap.String("url", req.URL.String()), zap.Int("statusCode", resp.StatusCode))
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

	start := time.Now()
	fields := []zap.Field{
		zap.String("method", method),
		zap.String("url", url),
	}
	if input != nil {
		fields = append(fields, zap.Any("request", input))
	}
	c.logger.Debug("mealie request", fields...)

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
		c.logger.Debug("mealie request failed", zap.String("method", method), zap.String("url", url), zap.Duration("duration", time.Since(start)), zap.Error(err))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := responseError(resp)
		c.logger.Debug("mealie response error", zap.String("method", method), zap.String("url", url), zap.Int("statusCode", resp.StatusCode), zap.Duration("duration", time.Since(start)), zap.Error(err))
		return err
	}
	if output == nil {
		c.logger.Debug("mealie response", zap.String("method", method), zap.String("url", url), zap.Int("statusCode", resp.StatusCode), zap.Duration("duration", time.Since(start)))
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		c.logger.Debug("mealie response decode failed", zap.String("method", method), zap.String("url", url), zap.Int("statusCode", resp.StatusCode), zap.Duration("duration", time.Since(start)), zap.Error(err))
		return err
	}
	c.logger.Debug("mealie response", zap.String("method", method), zap.String("url", url), zap.Int("statusCode", resp.StatusCode), zap.Duration("duration", time.Since(start)), zap.Any("response", output))
	return nil
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
