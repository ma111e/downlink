package adminserver

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"strconv"
	"time"

	"github.com/ma111e/downlink/pkg/utils"
)

//go:embed templates/*.tmpl templates/*.css
var templateFS embed.FS

// layoutCSS is the shared admin stylesheet, split out of layout.tmpl. It is
// injected via the styleCSS template func so it stays trusted (no CSS-context
// escaping) instead of being re-parsed as template text.
var layoutCSS = func() template.CSS {
	b, err := templateFS.ReadFile("templates/layout.css")
	if err != nil {
		panic(fmt.Sprintf("read embedded layout.css: %v", err))
	}
	return template.CSS(utils.StripCSSComments(string(b)))
}()

// pageRenderer is the subset of *template.Template the handlers use; it keeps
// render() decoupled from the concrete template type for testability.
type pageRenderer interface {
	Execute(wr io.Writer, data interface{}) error
}

var funcs = template.FuncMap{
	"comma":     comma,
	"shortTime": shortTime,
	"durMs":     durMs,
	"truncate":  truncate,
	"add":       func(a, b int) int { return a + b },
	"sub":       func(a, b int) int { return a - b },
	"ratioBar":  ratioBar,
	"styleCSS":  func() template.CSS { return layoutCSS },
}

// ratioBar renders a thin two-segment bar showing the uplink:downlink split.
// Widths are percentages so it scales to its CSS container.
func ratioBar(up, down int) template.HTML {
	total := up + down
	if total <= 0 {
		return template.HTML(`<div class="ratio"></div>`)
	}
	upPct := up * 100 / total
	return template.HTML(fmt.Sprintf(
		`<div class="ratio"><i class="u" style="width:%d%%"></i><i class="d" style="width:%d%%"></i></div>`,
		upPct, 100-upPct,
	))
}

// Each page is the shared layout plus one content template. Parsing them into
// separate sets avoids "content" name collisions between pages.
var (
	runsTmpl = template.Must(template.New("layout.tmpl").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.tmpl", "templates/runs.tmpl"))
	runTmpl = template.Must(template.New("layout.tmpl").Funcs(funcs).
		ParseFS(templateFS, "templates/layout.tmpl", "templates/run.tmpl"))
	refreshesTmpl = template.Must(template.New("layout.tmpl").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.tmpl", "templates/refreshes.tmpl"))
	refreshRunTmpl = template.Must(template.New("layout.tmpl").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.tmpl", "templates/refresh.tmpl"))
)

// comma formats an integer with thousands separators (e.g. 12345 -> "12,345").
func comma(n int) string {
	s := strconv.Itoa(n)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg, s = true, s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func shortTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

func durMs(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	if ms < 1000 {
		return strconv.FormatInt(ms, 10) + "ms"
	}
	return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "s"
}

func truncate(n int, s string) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
