package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/pulumi/pulumi-command/sdk/go/command/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/spf13/cobra"
)

var (
	//go:embed provision
	provision embed.FS

	suffix string
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

func parseTarget(s string) (u, h, p string, err error) {
	p = "22"
	if idx := strings.LastIndex(s, "@"); idx >= 0 {
		u = s[:idx]
		s = s[idx+1:]
	}
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		h, p = s[:idx], s[idx+1:]
	} else {
		h = s
	}
	if h == "" {
		return "", "", "", fmt.Errorf("invalid target %q: host is required", s)
	}
	if u == "" {
		u = os.Getenv("USER")
	}
	if u == "" {
		u = "root"
	}
	if u == "" {
		u = "ubuntu"
	}
	return u, h, p, nil
}

func url(u, p, h, s string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:5432/minhareceita%s", u, p, h, s)
}

var provisionCmd = &cobra.Command{
	Use:   "provision USER@IP",
	Short: "Provisions a PostgreSQL database on a remote Debian/Ubuntu server",
	Long: `Provisions PostgreSQL on a fresh Debian or Ubuntu server via SSH.

Requires:
  - pulumi CLI installed and available on $PATH
  - SSH access to the target server (key-based or SSH agent)
  - sudo access on the target server

Creates a "minhareceita" database with two users:
  - "etl" with full write access
  - "web" with read-only access

Passwords are randomly generated and printed to stdout.
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ini := time.Now()
		u, h, p, err := parseTarget(args[0])
		if err != nil {
			return err
		}
		port, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return fmt.Errorf("invalid port %q: %w", p, err)
		}
		etl, err := password()
		if err != nil {
			return fmt.Errorf("failed to generate etl password: %w", err)
		}
		web, err := password()
		if err != nil {
			return fmt.Errorf("failed to generate web password: %w", err)
		}

		ctx := context.Background()

		var buf bytes.Buffer
		t, err := template.ParseFS(provision, "provision/install.sh")
		if err != nil {
			return fmt.Errorf("failed to parse install template: %w", err)
		}
		if err := t.Execute(&buf, nil); err != nil {
			return fmt.Errorf("failed to render install template: %w", err)
		}
		sh := buf.String()

		buf.Reset()
		t, err = template.ParseFS(provision, "provision/setup.sql")
		if err != nil {
			return fmt.Errorf("failed to parse setup template: %w", err)
		}
		if err := t.Execute(&buf, struct{ EtlPass, WebPass, Suffix string }{etl, web, suffix}); err != nil {
			return fmt.Errorf("failed to render setup template: %w", err)
		}
		sql := fmt.Sprintf("sudo -u postgres psql <<'EOF'\n%s\nEOF", buf.String())

		fn := func(ctx *pulumi.Context) error {
			c := remote.ConnectionArgs{
				Host: pulumi.String(h),
				Port: pulumi.Float64Ptr(port),
				User: pulumi.StringPtr(u),
			}

			pg, err := remote.NewCommand(ctx, "install-postgresql", &remote.CommandArgs{
				Connection: c,
				Create:     pulumi.String(sh),
			})
			if err != nil {
				return err
			}

			_, err = remote.NewCommand(ctx, "setup-database", &remote.CommandArgs{
				Connection: c,
				Create:     pulumi.String(sql),
				Update:     pulumi.String(sql),
			}, pulumi.DependsOn([]pulumi.Resource{pg}))
			if err != nil {
				return err
			}

			return nil
		}

		dir, err := os.MkdirTemp("", "minha-receita-provision-*")
		if err != nil {
			return fmt.Errorf("failed to create workspace: %w", err)
		}
		defer func() {
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("could not remove workspace directory", "path", dir, "error", err)
			}
		}()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sig
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("could not remove workspace directory on signal", "path", dir, "error", err)
			}
			os.Exit(1)
		}()

		s, err := auto.NewStackInlineSource(ctx, "provision-"+h, "minha-receita-provision", fn,
			auto.WorkDir(dir),
			auto.EnvVars(map[string]string{
				"PULUMI_CONFIG_PASSPHRASE": "",
				"PULUMI_BACKEND_URL":       "file://" + dir,
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to create stack: %w", err)
		}

		res, err := s.Up(ctx, optup.ProgressStreams(os.Stdout))
		if err != nil {
			return fmt.Errorf("provisioning failed: %w\n%s", err, res.StdErr)
		}

		slog.Info("ETL credentials", "url", url("etl", etl, h, suffix))
		slog.Info("Web credentials", "url", url("web", web, h, suffix))
		slog.Info("provisioning complete", "elapsed", time.Since(ini))
		return nil
	},
}

func provisionCLI() *cobra.Command {
	provisionCmd.Flags().StringVarP(&suffix, "suffix", "s", "", "database name suffix")
	return provisionCmd
}
