package mealie

type CreateRecipe struct {
	Name string `json:"name"`
}

type CreateCategory struct {
	Name string `json:"name"`
}

type RecipeSearchResults struct {
	Items []RecipeSummary `json:"items"`
}

type CategoryResults struct {
	Items      []Organizer `json:"items"`
	Page       int         `json:"page"`
	TotalPages int         `json:"total_pages"`
}

type RecipeSummary struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Recipe struct {
	ID                 string             `json:"id,omitempty"`
	UserID             string             `json:"userId,omitempty"`
	HouseholdID        string             `json:"householdId,omitempty"`
	GroupID            string             `json:"groupId,omitempty"`
	Slug               string             `json:"slug,omitempty"`
	Name               string             `json:"name,omitempty"`
	RecipeYield        string             `json:"recipeYield,omitempty"`
	TotalTime          string             `json:"totalTime,omitempty"`
	PrepTime           string             `json:"prepTime,omitempty"`
	CookTime           string             `json:"cookTime,omitempty"`
	Description        string             `json:"description,omitempty"`
	RecipeCategory     []Organizer        `json:"recipeCategory,omitempty"`
	Tags               []Organizer        `json:"tags,omitempty"`
	OrgURL             string             `json:"orgURL,omitempty"`
	DateAdded          string             `json:"dateAdded,omitempty"`
	RecipeIngredient   []RecipeIngredient `json:"recipeIngredient,omitempty"`
	RecipeInstructions []RecipeStep       `json:"recipeInstructions,omitempty"`
	Notes              []RecipeNote       `json:"notes,omitempty"`
	Extras             map[string]any     `json:"extras,omitempty"`
}

type Organizer struct {
	ID      string `json:"id,omitempty"`
	GroupID string `json:"groupId,omitempty"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
}

type RecipeIngredient struct {
	Display      string `json:"display,omitempty"`
	Note         string `json:"note,omitempty"`
	OriginalText string `json:"originalText,omitempty"`
}

type RecipeStep struct {
	Title string `json:"title,omitempty"`
	Text  string `json:"text"`
}

type RecipeNote struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}
