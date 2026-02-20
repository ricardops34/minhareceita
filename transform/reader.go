package transform

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cuducos/go-cnpj"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/encoding/charmap"
)

var multipleSpaces = regexp.MustCompile(`\s{2,}`)
var yearMonthZipPattern = regexp.MustCompile(`^\d{4}-\d{2}\.zip$`)

func removeNulChar(r rune) rune {
	if r == '\x00' {
		return -1
	}
	return r
}

func cleanupColumn(s string) string {
	s = strings.Map(removeNulChar, s)
	s = multipleSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

type countReader struct {
	reader io.Reader
	read   int64
}

func (b *countReader) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += int64(n)
	return n, err
}

type reader struct {
	pth string
	src *source
}

func (c *reader) readFromReader(ctx context.Context, f io.Reader, bar *progressbar.ProgressBar, kv *kv) error {
	b := countReader{f, 0}
	r := csv.NewReader(charmap.ISO8859_15.NewDecoder().Reader(&b))
	r.Comma = c.src.sep
	if c.src.hasHeader {
		if _, err := r.Read(); err != nil {
			return fmt.Errorf("could not skip %s header: %w", c.pth, err)
		}
	}
	var prev int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			row, err := r.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("error reading %s: %w", c.pth, err)
			}
			if len(row) < 2 {
				return fmt.Errorf("unexpected row with %d columns in %s", len(row), c.src.prefix)
			}
			for n := range row {
				row[n] = cleanupColumn(row[n])
			}
			key := row[0]
			val := row[1:]
			if c.src.key == "imu" || c.src.key == "arb" || c.src.key == "pre" || c.src.key == "rea" {
				key = cnpj.Unmask(row[1])
				val = append([]string{row[0]}, row[2:]...)
			}
			if err := kv.put(c.src, key, val); err != nil {
				return fmt.Errorf("could not save %s line %v to badger: %w", c.src.prefix, row, err)
			}
			s := b.read - prev
			if bar != nil && s > 0 {
				if err := bar.Add64(s); err != nil {
					slog.Warn("could not update the progress bar", "error", err)
				}
			}
			prev = b.read

		}
	}
}

func (c *reader) readArchivedCSV(ctx context.Context, bar *progressbar.ProgressBar, kv *kv) error {
	a, err := zip.OpenReader(c.pth)
	if err != nil {
		return fmt.Errorf("could not open archive %s: %w", c.pth, err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			slog.Warn("could not close %s reader", "path", c.pth, "error", err)
		}
	}()
	var g errgroup.Group
	for _, z := range a.File {
		if bar != nil {
			bar.AddMax64(int64(z.UncompressedSize64))
		}
		if z.FileInfo().IsDir() {
			continue
		}
		f, err := z.Open()
		if err != nil {
			return fmt.Errorf("could not read %s from %s: %w", z.Name, c.pth, err)
		}
		g.Go(func() error {
			defer func() {
				if err := f.Close(); err != nil {
					slog.Warn("Could not close csv reader", "path", c.pth, "name", z.Name, "error", err)
				}
			}()
			return c.readFromReader(ctx, f, bar, kv)
		})
	}
	return g.Wait()
}

func (c *reader) readCSV(ctx context.Context, bar *progressbar.ProgressBar, kv *kv) error {
	f, err := os.Open(c.pth)
	if err != nil {
		return fmt.Errorf("could not open csv %s: %w", c.pth, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close csv reader", "path", c.pth, "error", err)
		}
	}()
	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("could not get stats for %s: %w", c.pth, err)
	}
	if bar != nil {
		bar.AddMax64(st.Size())
	}
	return c.readFromReader(ctx, f, bar, kv)
}

func findMainZIP(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if yearMonthZipPattern.MatchString(e.Name()) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("could not find YYYY-MM.zip in %s", dir)
}

func extractFile(z *zip.File, dir string, bar *progressbar.ProgressBar) error {
	f, err := os.Create(filepath.Join(dir, filepath.Base(z.Name)))
	if err != nil {
		return fmt.Errorf("could not create file for %s: %w", z.Name, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close extracted file", "name", z.Name, "error", err)
		}
	}()
	r, err := z.Open()
	if err != nil {
		return fmt.Errorf("could not open %s in archive: %w", z.Name, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("could not close archive entry", "name", z.Name, "error", err)
		}
	}()
	var dst io.Writer = f
	if bar != nil {
		dst = io.MultiWriter(f, bar)
	}
	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("could not extract %s: %w", z.Name, err)
	}
	return nil
}

func unzipMainArchive(pth, dir string, bar *progressbar.ProgressBar) error {
	a, err := zip.OpenReader(pth)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", pth, err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			slog.Warn("could not close archive", "path", pth, "error", err)
		}
	}()
	if bar != nil {
		var n int64
		for _, z := range a.File {
			if !z.FileInfo().IsDir() {
				n += int64(z.UncompressedSize64)
			}
		}
		bar.AddMax64(n - 1) // -1 to compensate for the initial max=1 from newProgressBar
	}
	for _, z := range a.File {
		if z.FileInfo().IsDir() {
			continue
		}
		if err := extractFile(z, dir, bar); err != nil {
			return err
		}
	}
	return nil
}

func loadIBGEMunicipalitiesFromURL(ctx context.Context, url string, src *source, bar *progressbar.ProgressBar, kv *kv) error {
	if bar != nil {
		defer func() {
			bar.AddMax(-1) // compensate for the extra byte added when creating the bar
		}()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("could not create request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not download ibge municipalities from %s: %w", url, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("could not close ibge municipalities response body", "url", url, "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ibge municipalities download from %s returned %s", url, resp.Status)
	}
	if bar != nil && resp.ContentLength > 0 {
		bar.AddMax64(resp.ContentLength)
	}
	r := reader{url, src}
	return r.readFromReader(ctx, resp.Body, bar, kv)
}

func loadCSVs(ctx context.Context, dir string, src *source, bar *progressbar.ProgressBar, kv *kv, del bool) error {
	if bar != nil {
		defer func() {
			bar.AddMax(-1) // compensate for the extra byte added when creating the bar
		}()
	}
	pths, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", dir, err)
	}
	pfx := src.prefix
	if src.filePrefix != "" {
		pfx = src.filePrefix
	}
	var g errgroup.Group
	for _, pth := range pths {
		if strings.HasPrefix(pth.Name(), pfx) {
			p := pth
			g.Go(func() error {
				pth := filepath.Join(dir, p.Name())
				r := reader{pth, src}
				var err error
				switch filepath.Ext(p.Name()) {
				case ".zip":
					err = r.readArchivedCSV(ctx, bar, kv)
				case ".csv":
					err = r.readCSV(ctx, bar, kv)
				default:
					return fmt.Errorf("unexpected file extension for %s", pth)
				}
				if err != nil {
					return err
				}
				if del {
					if e := os.Remove(pth); e != nil {
						slog.Warn("could not remove", "path", pth, "error", e)
					}
				}
				return nil
			})
		}
	}
	return g.Wait()
}
