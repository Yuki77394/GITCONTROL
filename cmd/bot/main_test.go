package main

import "testing"

func TestRedactMongoURI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "standard with password",
			in:   "mongodb://user:secretpass@host:27017/dbname",
			want: "mongodb://user:***@host:27017/dbname",
		},
		{
			name: "srv with password",
			in:   "mongodb+srv://user:secretpass@cluster.example.com/dbname",
			want: "mongodb+srv://user:***@cluster.example.com/dbname",
		},
		{
			name: "no credentials",
			in:   "mongodb://host:27017/dbname",
			want: "mongodb://host:27017/dbname",
		},
		{
			name: "username only no password",
			in:   "mongodb://user@host:27017/dbname",
			want: "mongodb://***@host:27017/dbname",
		},
		{
			name: "empty",
			in:   "",
			want: "(empty)",
		},
		{
			name: "invalid uri",
			in:   "not-a-uri",
			want: "(invalid-uri)",
		},
		{
			name: "atlas style with options",
			in:   "mongodb+srv://admin:S0me!Complex#Pass@cluster0.xxxxx.mongodb.net/mydb?retryWrites=true&w=majority",
			want: "mongodb+srv://admin:***@cluster0.xxxxx.mongodb.net/mydb?retryWrites=true&w=majority",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redactMongoURI(c.in)
			if got != c.want {
				t.Errorf("redactMongoURI(%q) = %q, want %q", c.in, got, c.want)
			}
			// Ensure the password is never present in the output.
			if c.in != "" && c.in != c.want {
				// Extract the password from the input to verify it's gone.
				if pw := extractPassword(c.in); pw != "" && containsStr(got, pw) {
					t.Errorf("redactMongoURI leaked password %q in output %q", pw, got)
				}
			}
		})
	}
}

func extractPassword(uri string) string {
	schemeEnd := indexOf(uri, "://")
	if schemeEnd < 0 {
		return ""
	}
	rest := uri[schemeEnd+3:]
	atIdx := indexOf(rest, "@")
	if atIdx < 0 {
		return ""
	}
	creds := rest[:atIdx]
	colonIdx := indexOf(creds, ":")
	if colonIdx < 0 {
		return ""
	}
	return creds[colonIdx+1:]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsStr(s, sub string) bool {
	return indexOf(s, sub) >= 0
}
