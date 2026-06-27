package utils

import "testing"

func TestStripCSSComments(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"removes block comment", "a{color:red;/* note */}", "a{color:red; }"},
		{"drops comment-only line", "a{}\n  /* heading */\nb{}", "a{}\nb{}"},
		{"keeps token boundary", "a:0/* x */0", "a:0 0"},
		{"ignores comment inside double-quoted url", `a{background:url("data:image/svg+xml,/*nope*/")}`, `a{background:url("data:image/svg+xml,/*nope*/")}`},
		{"ignores comment inside single-quoted string", "a{content:'/* keep */'}", "a{content:'/* keep */'}"},
		{"trims trailing whitespace", "a{}   \nb{}", "a{}\nb{}"},
		{"unterminated comment", "a{}/* dangling", "a{}"},
		{"escaped quote in string", `a{content:"\"/* x */"}`, `a{content:"\"/* x */"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StripCSSComments(c.in); got != c.want {
				t.Errorf("StripCSSComments(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
