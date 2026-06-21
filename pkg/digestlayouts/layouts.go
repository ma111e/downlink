package digestlayouts

// Layout names a graphical/layout theme for the digest HTML output. A layout is a
// full set of page templates living under the notification package's
// templates/<name>/ directory. Color palettes are a separate, client-side concern
// (see pkg/digestthemes and the in-page theme dropdown); this registry exists so the
// CLI can validate and list the layouts the server can render.
type Layout struct {
	Name        string
	Description string
}

const defaultLayout = "default"

var layouts = map[string]Layout{
	"default": {Name: "default", Description: "Default digest layout"},
	"emerald": {Name: "emerald", Description: "Default layout with a green accent (demo)"},
	"column":  {Name: "column", Description: "Column design system — paper-white Swiss banking"},
}

var order = []string{"default", "emerald", "column"}

// All returns all available layouts in display order.
func All() []Layout {
	result := make([]Layout, 0, len(order))
	for _, name := range order {
		result = append(result, layouts[name])
	}
	return result
}

// Valid reports whether name is a known layout.
func Valid(name string) bool {
	_, ok := layouts[name]
	return ok
}

// Get returns the Layout for the given name and whether it was found.
func Get(name string) (Layout, bool) {
	l, ok := layouts[name]
	return l, ok
}

// Default returns the name of the default layout, used when none is specified.
func Default() string {
	return defaultLayout
}
