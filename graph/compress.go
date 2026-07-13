package graph

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
)

func Compress(path string, bar *progressbar.ProgressBar) error {
	var c int64
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			i, err := d.Info()
			if err != nil {
				return err
			}
			c += i.Size()
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not calculate graph size: %w", err)
	}

	f, err := os.Create(path + ".tar.gz")
	if err != nil {
		return fmt.Errorf("could not create tar.gz file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close tar.gz file", "error", err)
		}
	}()

	var g *gzip.Writer
	if bar != nil {
		bar.ChangeMax64(c)
		g = gzip.NewWriter(io.MultiWriter(f, bar))
	} else {
		g = gzip.NewWriter(f)
	}
	defer func() {
		if err := g.Close(); err != nil {
			slog.Warn("could not close gzip writer", "error", err)
		}
	}()

	t := tar.NewWriter(g)
	defer func() {
		if err := t.Close(); err != nil {
			slog.Warn("could not close tar writer", "error", err)
		}
	}()

	n := filepath.Base(path)
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		i, err := d.Info()
		if err != nil {
			return err
		}

		r, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}

		h, err := tar.FileInfoHeader(i, "")
		if err != nil {
			return err
		}
		h.Name = filepath.Join(n, r)

		if err := t.WriteHeader(h); err != nil {
			return err
		}

		f, err := os.Open(p)
		if err != nil {
			return err
		}
		if _, err := io.Copy(t, f); err != nil {
			return errors.Join(err, f.Close())
		}
		if err := f.Close(); err != nil {
			slog.Warn("could not close file", "path", p, "error", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("could not compress graph directory: %w", err)
	}

	if err := t.Close(); err != nil {
		return fmt.Errorf("could not close tar writer: %w", err)
	}
	if err := g.Close(); err != nil {
		return fmt.Errorf("could not close gzip writer: %w", err)
	}

	return os.RemoveAll(path)
}

// Decompress extracts a .tar.gz archive into a directory next to it (without the
// .tar.gz suffix) and returns the path to that directory. The archive is kept.
func Decompress(pth string) (string, error) {
	dir := strings.TrimSuffix(pth, ".tar.gz")
	f, err := os.Open(pth)
	if err != nil {
		return "", fmt.Errorf("could not open tar.gz file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close tar.gz file", "error", err)
		}
	}()
	g, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("could not create gzip reader: %w", err)
	}
	defer func() {
		if err := g.Close(); err != nil {
			slog.Warn("could not close gzip reader", "error", err)
		}
	}()
	t := tar.NewReader(g)
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("could not read tar header: %w", err)
		}
		target := filepath.Join(filepath.Dir(dir), h.Name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", fmt.Errorf("could not create directory %s: %w", filepath.Dir(target), err)
		}
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", fmt.Errorf("could not create directory %s: %w", target, err)
			}
			continue
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return "", fmt.Errorf("could not create file %s: %w", target, err)
		}
		if _, err := io.Copy(out, t); err != nil {
			return "", fmt.Errorf("could not write file %s: %w", target, errors.Join(err, out.Close()))
		}
		if err := out.Close(); err != nil {
			slog.Warn("could not close file", "path", target, "error", err)
		}
	}
	return dir, nil
}
