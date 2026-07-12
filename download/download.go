package download

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

var taxRegimeFiles = []string{
	"entidades-imunes-e-isentas.zip",
	"entidades-lucro-arbitrado.zip",
	"entidades-lucro-presumido.zip",
	"entidades-lucro-real.zip",
}

func validateYearMonth(ym string) error {
	if _, err := time.Parse("2006-01", ym); err != nil {
		if _, err := time.Parse("200601", ym); err != nil {
			return fmt.Errorf("invalid year-month %q, expected YYYY-MM or YYYYMM", ym)
		}
	}
	return nil
}

func normalizeYearMonth(ym string) string {
	if t, err := time.Parse("200601", ym); err == nil {
		return t.Format("2006-01")
	}
	return ym
}

func isZipFile(e entry) bool {
	return e.Collection == nil && strings.EqualFold(filepath.Ext(e.DisplayName), ".zip")
}

func downloadFederalRevenue(c *webdav, ym, dir string) error {
	ls, err := c.list(ym)
	if err != nil {
		return fmt.Errorf("could not list files for %s: %w", ym, err)
	}
	var zs []entry
	for _, e := range ls {
		if isZipFile(e) {
			zs = append(zs, e)
		}
	}
	if len(zs) == 0 {
		return fmt.Errorf("no files found in %s", ym)
	}
	var total int64
	for _, f := range zs {
		total += f.ContentLength
	}
	bar := progressbar.NewOptions(
		int(total),
		progressbar.OptionSetDescription(fmt.Sprintf("Downloading %s", ym)),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionFullWidth(),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)
	if err := bar.RenderBlank(); err != nil {
		return fmt.Errorf("could not start progress bar: %w", err)
	}
	var g errgroup.Group
	for _, e := range zs {
		g.Go(func() error {
			n := e.DisplayName
			pth := filepath.Join(dir, n)

			f, err := os.Create(pth)
			if err != nil {
				return fmt.Errorf("could not create %s: %w", pth, err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					slog.Warn("could not close file", "file", n, "error", err)
				}
			}()
			_, err = c.download(ym+"/"+n, io.MultiWriter(f, bar))
			if err != nil {
				return fmt.Errorf("could not download %s: %w", n, err)
			}

			return nil
		})
	}
	return g.Wait()
}

func downloadTaxRegime(c *webdav, dir string) error {
	ls, err := c.list("")
	if err != nil {
		return fmt.Errorf("could not list tax regime files: %w", err)
	}
	ok := make(map[string]bool)
	for _, e := range ls {
		if isZipFile(e) {
			ok[e.DisplayName] = true
		}
	}
	var todo []string
	for _, f := range taxRegimeFiles {
		if ok[f] {
			todo = append(todo, f)
		}
	}
	if len(todo) == 0 {
		return fmt.Errorf("no tax regime files found")
	}
	var g errgroup.Group
	for _, n := range todo {
		g.Go(func() error {
			pth := filepath.Join(dir, n)

			f, err := os.Create(pth)
			if err != nil {
				return fmt.Errorf("could not create %s: %w", pth, err)
			}
			defer func() {
				if err := f.Close(); err != nil {
					slog.Warn("could not close file", "file", n, "error", err)
				}
			}()
			_, err = c.download(n, f)
			if err != nil {
				return fmt.Errorf("could not download %s: %w", n, err)
			}

			return nil
		})
	}
	return g.Wait()
}

func downloadNationalTreasure(dir string) error {
	slog.Info("Downloading tabmun from the National Treasure…")
	urls, err := ckanURLs(nationalTreasureBase, nationalTreasurePkgID)
	if err != nil {
		return fmt.Errorf("error gathering resources for national treasure: %w", err)
	}
	for _, u := range urls {
		pth := filepath.Join(dir, filepath.Base(u))

		_, err := downloadURL(u, pth)
		if err != nil {
			return fmt.Errorf("error downloading %s: %w", u, err)
		}

	}
	return nil
}

// Download fetches all source files for the given year-month and saves them in
// the given directory. It downloads the CNPJ data from the Federal Revenue via
// WebDAV, the tax regime files, and the IBGE municipalities CSV from the
// National Treasure.
func Download(dir, ym string) error {
	if err := validateYearMonth(ym); err != nil {
		return err
	}
	ym = normalizeYearMonth(ym)
	i, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory %s does not exist: %w", dir, err)
	}
	if !i.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	r := newClient(cnpjToken)
	slog.Info("Downloading CNPJ files from the Federal Revenue…", "month", ym)
	if err := downloadFederalRevenue(r, ym, dir); err != nil {
		return fmt.Errorf("error downloading CNPJ files: %w", err)
	}
	t := newClient(taxRegimeToken)
	if err := downloadNationalTreasure(dir); err != nil {
		return fmt.Errorf("error downloading national treasure files: %w", err)
	}
	slog.Info("Downloading tax regime files from the Federal Revenue…")
	if err := downloadTaxRegime(t, dir); err != nil {
		return fmt.Errorf("error downloading tax regime files: %w", err)
	}
	ls, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range ls {
			if !e.IsDir() {
				if _, err := time.Parse("2006-01", e.Name()); err == nil {
					if e.Name() != ym {
						os.Remove(filepath.Join(dir, e.Name()))
					}
				}
			}
		}
	}
	_, err = os.Create(filepath.Join(dir, ym))
	return err
}
