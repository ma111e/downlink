package utils

import "testing"

func TestJoinURL(t *testing.T) {
	cases := []struct {
		base     string
		segments []string
		want     string
	}{
		{"https://user.github.io", []string{"digests", "x.html"}, "https://user.github.io/digests/x.html"},
		{"https://user.github.io/", []string{"digests", ""}, "https://user.github.io/digests"},
		{"", []string{"digests", "x.html"}, "/digests/x.html"},
		{"", []string{"", ""}, "/"},
		{"https://feeds.example.com", []string{"feeds", "my-feed"}, "https://feeds.example.com/feeds/my-feed"},
	}
	for _, c := range cases {
		if got := JoinURL(c.base, c.segments...); got != c.want {
			t.Errorf("JoinURL(%q, %v) = %q, want %q", c.base, c.segments, got, c.want)
		}
	}
}

func TestResolveLink(t *testing.T) {
	cases := []struct {
		name string
		base string
		link string
		want string
	}{
		{"empty base passthrough", "", "/posts/1", "/posts/1"},
		{"absolute link passthrough", "https://feeds.example.com", "https://other.example/x", "https://other.example/x"},
		{"relative link joined", "https://feeds.example.com", "/posts/1", "https://feeds.example.com/posts/1"},
		{"relative link without slash joined", "https://feeds.example.com", "posts/1", "https://feeds.example.com/posts/1"},
	}
	for _, c := range cases {
		if got := ResolveLink(c.base, c.link); got != c.want {
			t.Errorf("%s: ResolveLink(%q, %q) = %q, want %q", c.name, c.base, c.link, got, c.want)
		}
	}
}
