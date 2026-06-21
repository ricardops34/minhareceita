package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

const (
	sshPort    = "22"
	webEnvFile = "/etc/minha-receita/env.web"
	etlEnvFile = "/etc/minha-receita/env.etl"
)

var (
	//go:embed provision
	provision embed.FS

	suffix string
	domain string
)

func password() (string, error) {
	s := make([]byte, 1)
	if _, err := rand.Read(s); err != nil {
		return "", err
	}
	n := int(s[0])%97 + 32 // random length in [32, 128]
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type target struct {
	user, host, port string
}

func newTarget(args []string) (target, error) {
	if len(args) != 1 {
		return target{}, fmt.Errorf("expected exactly one target in the format USER@IP")
	}
	port := sshPort
	in := args[0]
	var user, host string
	if idx := strings.LastIndex(in, "@"); idx >= 0 {
		user = in[:idx]
		in = in[idx+1:]
	}
	if idx := strings.LastIndex(in, ":"); idx >= 0 {
		host, port = in[:idx], in[idx+1:]
	} else {
		host = in
	}
	if host == "" {
		return target{}, fmt.Errorf("invalid target %q: host is required", in)
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	if user == "" {
		user = "root"
	}
	if user == "" {
		user = "ubuntu"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return target{}, fmt.Errorf("invalid port %q: %w", port, err)
	}
	return target{user: user, host: host, port: port}, nil
}

func dbURL(user, pass, host, suffix string) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:5432", host),
		Path:   fmt.Sprintf("minhareceita%s", suffix),
	}
	return u.String()
}

func ssh(ctx context.Context, w io.Writer, t target, sh string) error {
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	if t.port != sshPort {
		args = append(args, "-p", t.port)
	}
	args = append(args, fmt.Sprintf("%s@%s", t.user, t.host), "bash -s")
	c := exec.CommandContext(ctx, "ssh", args...)
	c.Stdin = strings.NewReader(sh)
	c.Stdout = w
	c.Stderr = os.Stderr
	return c.Run()
}

type tmplCtx struct {
	Web, Suffix, Host, Domain string
	CacheSize, BloomSize      int
}

func (d *tmplCtx) render(name string) (string, error) {
	var buf bytes.Buffer
	t, err := template.ParseFS(provision, name)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", name, err)
	}
	if err := t.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("failed to render %s: %w", name, err)
	}
	return buf.String(), nil
}

func renderSQL(etl, web, suffix string) (string, error) {
	var buf bytes.Buffer
	t, err := template.ParseFS(provision, "provision/db.sql")
	if err != nil {
		return "", fmt.Errorf("failed to parse provision/db.sql: %w", err)
	}
	if err := t.Execute(&buf, struct{ ETL, Web, Suffix string }{etl, web, suffix}); err != nil {
		return "", fmt.Errorf("failed to render provision/db.sql: %w", err)
	}
	return buf.String(), nil
}

func ctxWithSignal() (context.Context, context.CancelFunc) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	return ctx, cancel
}

func readEnv(ctx context.Context, t target, pth string) (string, bool, error) {
	var buf bytes.Buffer
	sh := fmt.Sprintf("if test -f %s; then cat %s; fi", pth, pth)
	if err := ssh(ctx, &buf, t, sh); err != nil {
		return "", false, fmt.Errorf("failed to read %s: %w", pth, err)
	}
	out := strings.TrimSpace(buf.String())
	if out == "" {
		return "", false, nil
	}
	if !strings.HasPrefix(out, "DATABASE_URL=") {
		return "", false, fmt.Errorf("invalid %s format", pth)
	}
	return out[13:], true, nil
}

func writeEnv(ctx context.Context, t target, pth, content string) error {
	sh := fmt.Sprintf("sudo mkdir -p %q && printf '%%s' %q | sudo tee %s > /dev/null && sudo chmod 600 %s", dirOf(pth), content, pth, pth)
	return ssh(ctx, os.Stdout, t, sh)
}

func dirOf(p string) string {
	idx := strings.LastIndex(p, "/")
	if idx < 0 {
		return "."
	}
	return p[:idx]
}

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Provisions a remote server for Minha Receita",
	Long: `Commands to provision a remote Debian/Ubuntu server for Minha Receita.

Requires SSH access (key-based or SSH agent) and sudo on the target server.
`,
}

var provisionDBCmd = &cobra.Command{
	Use:   "db USER@IP",
	Short: "Provisions a PostgreSQL database on a remote server",
	Long: `Installs and configures PostgreSQL on a remote Debian/Ubuntu server,
creates the minhareceita database, and sets up etl (write) and web (read-only)
users.

Credentials are saved on the server so the db and web subcommands are
idempotent. The etl (write) credential is also printed to stdout on each run.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ini := time.Now()
		t, err := newTarget(args)
		if err != nil {
			return err
		}
		ctx, cancel := ctxWithSignal()
		defer cancel()

		etl, ok, err := readEnv(ctx, t, etlEnvFile)
		if err != nil {
			return err
		}
		if !ok {
			etl, err = password()
			if err != nil {
				return fmt.Errorf("failed to generate etl password: %w", err)
			}
			slog.Info("ETL credentials", "url", dbURL("etl", etl, t.host, suffix))
		}

		web, ok, err := readEnv(ctx, t, webEnvFile)
		if err != nil {
			return err
		}
		if !ok {
			web, err = password()
			if err != nil {
				return fmt.Errorf("failed to generate web password: %w", err)
			}
			slog.Info("Web credentials", "url", dbURL("web", web, t.host, suffix))
		}

		sh, err := template.ParseFS(provision, "provision/db.sh")
		if err != nil {
			return err
		}
		var b bytes.Buffer
		if err := sh.Execute(&b, nil); err != nil {
			return err
		}
		sql, err := renderSQL(etl, web, suffix)
		if err != nil {
			return err
		}
		sql = fmt.Sprintf("sudo -u postgres psql <<'EOF'\n%s\nEOF", sql)

		slog.Info("Installing PostgreSQL", "host", t.host)
		if err := ssh(ctx, os.Stdout, t, b.String()); err != nil {
			return fmt.Errorf("failed to install PostgreSQL: %w", err)
		}
		slog.Info("Setting up database and users", "host", t.host)
		if err := ssh(ctx, os.Stdout, t, sql); err != nil {
			return fmt.Errorf("failed to set up database: %w", err)
		}

		d := tmplCtx{Web: web, Suffix: suffix, Host: t.host}
		cfg, err := d.render("provision/env")
		if err != nil {
			return err
		}
		slog.Info("Saving web credentials", "path", webEnvFile)
		if err := writeEnv(ctx, t, webEnvFile, cfg); err != nil {
			return fmt.Errorf("failed to save web credentials: %w", err)
		}
		d = tmplCtx{Web: etl, Suffix: suffix, Host: t.host}
		cfg, err = d.render("provision/env")
		if err != nil {
			return err
		}
		slog.Info("Saving etl credentials", "path", etlEnvFile)
		if err := writeEnv(ctx, t, etlEnvFile, cfg); err != nil {
			return fmt.Errorf("failed to save etl credentials: %w", err)
		}

		slog.Info("Database provisioning complete", "elapsed", time.Since(ini))
		return nil
	},
}

var provisionWebCmd = &cobra.Command{
	Use:   "web USER@IP",
	Short: "Deploys the web and graph APIs on a remote server",
	Long: `Deploys the main web API and the graph API on a remote server.

Requires the db subcommand to have run first, since the web APIs need the
database credentials saved by that step. If Docker is not present on the server,
it is installed automatically.

The default domain is minhareceita.org and the graph API is served at
grafo.minhareceita.org. Use --domain to override.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ini := time.Now()
		t, err := newTarget(args)
		if err != nil {
			return err
		}
		ctx, cancel := ctxWithSignal()
		defer cancel()

		var b bytes.Buffer
		c := fmt.Sprintf("test -f %s && cat %s", webEnvFile, webEnvFile)
		if err := ssh(ctx, &b, t, c); err != nil {
			return fmt.Errorf("database credentials not found on server; run `minha-receita provision db %s` first", args[0])
		}
		out := b.String()
		if !strings.Contains(out, "DATABASE_URL=") {
			return fmt.Errorf("database credentials file is incomplete on server; run `minha-receita provision db %s` first", args[0])
		}

		sh, err := template.ParseFS(provision, "provision/web.sh")
		if err != nil {
			return err
		}
		b.Reset()
		if err := sh.Execute(&b, nil); err != nil {
			return err
		}
		slog.Info("Ensuring Docker is installed", "host", t.host)
		if err := ssh(ctx, os.Stdout, t, b.String()); err != nil {
			return fmt.Errorf("failed to install Docker: %w", err)
		}

		d := tmplCtx{Web: "", Suffix: "", Host: t.host, Domain: domain, CacheSize: cacheSize, BloomSize: bloomSize}
		dir := "/etc/minha-receita"
		compose, err := template.ParseFS(provision, "provision/compose.yml")
		if err != nil {
			return err
		}
		b.Reset()
		if err := compose.Execute(&b, nil); err != nil {
			return err
		}
		s := b.String()
		nginx, err := d.render("provision/nginx.conf")
		if err != nil {
			return err
		}
		cfg, err := d.render("provision/env")
		if err != nil {
			return err
		}
		slog.Info("Uploading compose files", "host", t.host)
		if err := ssh(ctx, os.Stdout, t, fmt.Sprintf("sudo mkdir -p %s && sudo rm -rf %s/compose.yml %s/nginx.conf", dir, dir, dir)); err != nil {
			return fmt.Errorf("failed to prepare remote directory: %w", err)
		}
		if err := ssh(ctx, os.Stdout, t, fmt.Sprintf("cat > /tmp/compose.yml <<'EOF'\n%s\nEOF\nsudo mv /tmp/compose.yml %s/compose.yml", s, dir)); err != nil {
			return fmt.Errorf("failed to upload compose.yml: %w", err)
		}
		if err := ssh(ctx, os.Stdout, t, fmt.Sprintf("cat > /tmp/nginx.conf <<'EOF'\n%s\nEOF\nsudo mv /tmp/nginx.conf %s/nginx.conf", nginx, dir)); err != nil {
			return fmt.Errorf("failed to upload nginx.conf: %w", err)
		}
		if err := ssh(ctx, os.Stdout, t, fmt.Sprintf("cat > /tmp/env.web <<'EOF'\n%s\nEOF\nsudo mv /tmp/env.web %s", cfg, webEnvFile)); err != nil {
			return fmt.Errorf("failed to upload web env: %w", err)
		}

		slog.Info("Deploying web and graph APIs", "host", t.host)
		if err := ssh(ctx, os.Stdout, t, fmt.Sprintf("cd %s && sudo docker compose --env-file %s up -d", dir, webEnvFile)); err != nil {
			return fmt.Errorf("failed to deploy web and graph APIs: %w", err)
		}

		slog.Info("Web and graph deployment complete", "elapsed", time.Since(ini))
		return nil
	},
}

func provisionCLI() *cobra.Command {
	provisionCmd.AddCommand(provisionDBCmd, provisionWebCmd)
	provisionDBCmd.Flags().StringVarP(&suffix, "suffix", "s", "", "database name suffix")
	provisionWebCmd.Flags().StringVarP(&domain, "domain", "d", "minhareceita.org", "domain under which the APIs are served")
	provisionWebCmd.Flags().IntVarP(
		&cacheSize,
		"cache-size",
		"c",
		defaultCacheSize,
		"API cache size in MB",
	)
	provisionWebCmd.Flags().IntVarP(
		&bloomSize,
		"bloom-size",
		"b",
		defaultBloomSize,
		"API bloom filter size in MB",
	)
	return provisionCmd
}
