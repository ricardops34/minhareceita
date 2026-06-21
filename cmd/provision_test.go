package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	t.Parallel()
	user := os.Getenv("USER")
	if user == "" {
		user = "root"
	}

	for _, c := range []struct {
		in               string
		user, host, port string
		fail             string
	}{
		{in: "root@10.0.0.1", user: "root", host: "10.0.0.1", port: "22"},
		{in: "root@10.0.0.1:2222", user: "root", host: "10.0.0.1", port: "2222"},
		{in: "ubuntu@host:42", user: "ubuntu", host: "host", port: "42"},
		{in: "10.0.0.1", user: user, host: "10.0.0.1", port: "22"},
		{in: "10.0.0.1:2222", user: user, host: "10.0.0.1", port: "2222"},
		{in: "user@host", user: "user", host: "host", port: "22"},
		{in: "host", user: user, host: "host", port: "22"},
		{in: "", fail: "host is required"},
		{in: "user@", fail: "host is required"},
		{in: ":2222", fail: "host is required"},
	} {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			target, err := newTarget([]string{c.in})
			if c.fail != "" {
				if err == nil || !strings.Contains(err.Error(), c.fail) {
					t.Errorf("expected an error when parsing %q, got %v", c.in, err)
				}
				return
			}
			if err != nil {
				t.Errorf("expected no error when parsing %q, got %v", c.in, err)
				return
			}
			if target.user != c.user || target.host != c.host || target.port != c.port {
				t.Errorf("expected (%q,%q,%q) when parsing %q, got (%q,%q,%q)", c.user, c.host, c.port, c.in, target.user, target.host, target.port)
			}
		})
	}
}

func TestDBURL(t *testing.T) {
	t.Parallel()
	u := dbURL("web", "secret", "localhost", "_test")
	want := "postgres://web:secret@localhost:5432/minhareceita_test"
	if u != want {
		t.Errorf("expected %q, got %q", want, u)
	}
}

func TestPassword(t *testing.T) {
	t.Parallel()
	p, err := password()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(p) < 32 {
		t.Errorf("expected password length >= 32, got %d", len(p))
	}
}
