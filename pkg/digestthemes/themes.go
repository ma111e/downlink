package digestthemes

// Theme defines a named visual style for the digest HTML output.
type Theme struct {
	Name        string
	Description string
	Vars        map[string]string // CSS variable overrides; nil means use template defaults (dark)
}

var themes = map[string]Theme{
	"dark": {
		Name:        "dark",
		Description: "Dark navy/charcoal (default)",
		Vars:        nil,
	},
	"light": {
		Name:        "light",
		Description: "Light background with dark text",
		Vars: map[string]string{
			"--bg":         "#f5f6f8",
			"--surface":    "#ffffff",
			"--surface2":   "#eef0f3",
			"--border":     "#d8dbe2",
			"--border2":    "#c4c8d2",
			"--accent":     "#3b5bdb",
			"--accent-dim": "rgba(59,91,219,.10)",
			"--text":       "#343a40",
			"--heading":    "#1a1d23",
			"--muted":      "#868e96",
			"--muted2":     "#6c757d",
			"--link":       "#1c7ed6",
			"--shadow":     "0 1px 3px rgba(0,0,0,.12)",
		},
	},
	"solarized": {
		Name:        "solarized",
		Description: "Solarized dark palette",
		Vars: map[string]string{
			"--bg":         "#002b36",
			"--surface":    "#073642",
			"--surface2":   "#0d3d4a",
			"--border":     "#144f5e",
			"--border2":    "#1a6070",
			"--accent":     "#268bd2",
			"--accent-dim": "rgba(38,139,210,.15)",
			"--text":       "#839496",
			"--heading":    "#eee8d5",
			"--muted":      "#586e75",
			"--muted2":     "#657b83",
			"--link":       "#2aa198",
			"--shadow":     "0 1px 3px rgba(0,0,0,.5)",
		},
	},
	"nord": {
		Name:        "nord",
		Description: "Nord arctic palette",
		Vars: map[string]string{
			"--bg":         "#2e3440",
			"--surface":    "#3b4252",
			"--surface2":   "#434c5e",
			"--border":     "#4c566a",
			"--border2":    "#5a6275",
			"--accent":     "#88c0d0",
			"--accent-dim": "rgba(136,192,208,.15)",
			"--text":       "#d8dee9",
			"--heading":    "#eceff4",
			"--muted":      "#4c566a",
			"--muted2":     "#616e88",
			"--link":       "#81a1c1",
			"--shadow":     "0 1px 3px rgba(0,0,0,.45)",
		},
	},
	"colorblind": {
		Name:        "colorblind",
		Description: "Light, colorblind-safe (Okabe-Ito)",
		Vars: map[string]string{
			// Light-theme neutrals, reused verbatim.
			"--bg":       "#efeeea",
			"--surface":  "#fafaf8",
			"--surface2": "#e8e7e2",
			"--border":   "#d3d2cc",
			"--border2":  "#b8b7b0",
			"--text":     "#0d0c0a",
			"--text2":    "#273f4f",
			"--text3":    "#5c6e78",
			"--dot":      "rgba(0,0,0,.035)",
			// Colorblind-safe accents (Okabe-Ito, darkened for AA on the cream bg).
			"--cyan":             "#0072b2", // accent/links: Okabe blue
			"--green":            "#1a7a4f", // bluish-green (no pure green)
			"--violet":           "#7a4fa0", // purple
			"--must":             "#c44601", // vermillion
			"--should":           "#8a6d00", // dark gold
			"--may":              "#2f79a5", // teal-blue
			"--cat-research":     "#0072b2", // blue
			"--cat-advisory":     "#c44601", // vermillion
			"--cat-news":         "#2b6c8f", // dark sky
			"--cat-opinion":      "#a23b7a", // reddish-purple
			"--cat-guide":        "#1a7a4f", // bluish-green
			"--cat-commercial":   "#8a6d00", // gold
			"--cat-sponsored":    "#7a4fa0", // purple
			"--cat-announcement": "#00786b", // teal
		},
	},
}

var order = []string{"dark", "light", "solarized", "nord", "colorblind"}

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
