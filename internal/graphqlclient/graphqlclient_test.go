package graphqlclient

import "testing"

func TestDeriveGraphQLEndpoint(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "https://api.github.com/graphql"},
		{"https://api.github.com", "https://api.github.com/graphql"},
		{"https://api.github.com/", "https://api.github.com/graphql"},
		{"https://github.example.com/api/v3", "https://github.example.com/api/graphql"},
		{"https://github.example.com/api/v3/", "https://github.example.com/api/graphql"},
		{"https://custom.example.com/api", "https://custom.example.com/api/graphql"},
	}
	for _, c := range cases {
		got := DeriveGraphQLEndpoint(c.in)
		if got != c.want {
			t.Errorf("DeriveGraphQLEndpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
