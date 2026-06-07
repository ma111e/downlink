package digestthemes

// Theme defines a named visual style for the digest HTML output. The actual
// colors live in the templates' html[data-theme="<name>"] CSS blocks; this
// registry exists for CLI validation and listing.
type Theme struct {
	Name        string
	Description string
}

var themes = map[string]Theme{
	"dark":       {Name: "dark", Description: "Dark navy/charcoal (default)"},
	"light":      {Name: "light", Description: "Warm cream background, dark text"},
	"contrast":   {Name: "contrast", Description: "Maximum-contrast black & white"},
	"mono":       {Name: "mono", Description: "Grayscale, no chroma"},
	"colorblind": {Name: "colorblind", Description: "Light, colorblind-safe (Okabe-Ito)"},
	"pastel":     {Name: "pastel", Description: "Soft pastel cream/mint/coral, dark text"},
}

var order = []string{"dark", "light", "contrast", "mono", "colorblind", "pastel"}

// All returns all available themes in display order.
func All() []Theme {
	result := make([]Theme, 0, len(order))
	for _, name := range order {
		result = append(result, themes[name])
	}
	return result
}

// Valid reports whether name is a known theme.
func Valid(name string) bool {
	_, ok := themes[name]
	return ok
}

// Get returns the Theme for the given name and whether it was found.
func Get(name string) (Theme, bool) {
	t, ok := themes[name]
	return t, ok
}
