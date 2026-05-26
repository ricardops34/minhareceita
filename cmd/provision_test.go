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
			u, h, p, err := parseTarget(c.in)
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
			if u != c.user || h != c.host || p != c.port {
				t.Errorf("expected (%q,%q,%q) when parsing %q, got (%q,%q,%q)", c.user, c.host, c.port, c.in, u, h, p)
			}
		})
	}
}
