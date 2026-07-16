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

var cnpjFiles = func() []string {
	files := []string{
		"Cnaes.zip",
		"Motivos.zip",
		"Municipios.zip",
		"Naturezas.zip",
		"Paises.zip",
		"Qualificacoes.zip",
		"Simples.zip",
	}
	for _, prefix := range []string{"Empresas", "Estabelecimentos", "Socios"} {
		for i := range 10 {
			files = append(files, fmt.Sprintf("%s%d.zip", prefix, i))
		}
	}
	return files
}()

func listWithFallback(c *webdav, dir string, names []string) ([]entry, error) {
	ls, err := c.list(dir)
	if err == nil {
		return ls, nil
	}
	slog.Warn("WebDAV listing failed; checking known files individually", "directory", dir, "error", err)
	ls, fallbackErr := c.listFilesByName(dir, names)
	if fallbackErr != nil {
		return nil, fmt.Errorf("WebDAV listing failed (%v) and file discovery fallback failed: %w", err, fallbackErr)
	}
	return ls, nil
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

const downloadAttempts = 5

func downloadEntry(c *webdav, dir, remoteDir string, e entry, bar *progressbar.ProgressBar) error {
	name := e.DisplayName
	localPath := filepath.Join(dir, name)
	remotePath := strings.TrimPrefix(remoteDir+"/"+name, "/")

	var initialSize int64
	if info, err := os.Stat(localPath); err == nil {
		switch {
		case e.ContentLength > 0 && info.Size() == e.ContentLength:
			if err := bar.Add(int(info.Size())); err != nil {
				return fmt.Errorf("could not update progress for %s: %w", name, err)
			}
			slog.Info("File already downloaded; skipping", "file", name)
			return nil
		case e.ContentLength > 0 && info.Size() < e.ContentLength:
			initialSize = info.Size()
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not inspect %s: %w", localPath, err)
	}
	if initialSize > 0 {
		if err := bar.Add(int(initialSize)); err != nil {
			return fmt.Errorf("could not update progress for %s: %w", name, err)
		}
		slog.Info("Resuming partial download", "file", name, "offset", initialSize, "size", e.ContentLength)
	}

	var lastErr error
	for attempt := range downloadAttempts {
		offset := int64(0)
		if info, err := os.Stat(localPath); err == nil && e.ContentLength > 0 && info.Size() < e.ContentLength {
			offset = info.Size()
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not inspect %s: %w", localPath, err)
		}

		flags := os.O_CREATE | os.O_WRONLY
		if offset > 0 {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(localPath, flags, 0o644)
		if err != nil {
			return fmt.Errorf("could not open %s: %w", localPath, err)
		}
		_, downloadErr := c.downloadFrom(remotePath, offset, io.MultiWriter(f, bar))
		closeErr := f.Close()
		if downloadErr == nil && closeErr != nil {
			downloadErr = fmt.Errorf("could not close %s: %w", localPath, closeErr)
		}
		if downloadErr == nil && e.ContentLength > 0 {
			info, statErr := os.Stat(localPath)
			if statErr != nil {
				downloadErr = fmt.Errorf("could not verify %s: %w", localPath, statErr)
			} else if info.Size() != e.ContentLength {
				downloadErr = fmt.Errorf("incomplete file: got %d bytes, expected %d", info.Size(), e.ContentLength)
			}
		}
		if downloadErr == nil {
			return nil
		}
		lastErr = downloadErr
		if attempt+1 < downloadAttempts {
			slog.Warn("Download interrupted; retrying", "file", name, "attempt", attempt+1, "error", downloadErr)
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return fmt.Errorf("could not download %s after %d attempts: %w", name, downloadAttempts, lastErr)
}

func downloadFederalRevenue(c *webdav, ym, dir string) error {
	ls, err := listWithFallback(c, ym, cnpjFiles)
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
		progressbar.OptionSetDescription("Downloading files from the Federal Revenue…"),
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
			return downloadEntry(c, dir, ym, e, bar)
		})
	}
	return g.Wait()
}

func downloadTaxRegime(c *webdav, dir string) error {
	ls, err := listWithFallback(c, "", taxRegimeFiles)
	if err != nil {
		return fmt.Errorf("could not list tax regime files: %w", err)
	}
	sz := make(map[string]int64)
	for _, e := range ls {
		if isZipFile(e) {
			sz[e.DisplayName] = e.ContentLength
		}
	}
	var todo []string
	var total int64
	for _, f := range taxRegimeFiles {
		if s, ok := sz[f]; ok {
			todo = append(todo, f)
			total += s
		}
	}
	if len(todo) == 0 {
		return fmt.Errorf("no tax regime files found")
	}
	bar := progressbar.NewOptions(
		int(total),
		progressbar.OptionSetDescription("Downloading tax regime files from the Federal Revenue…"),
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
	for _, n := range todo {
		g.Go(func() error {
			return downloadEntry(c, dir, "", entry{DisplayName: n, ContentLength: sz[n]}, bar)
		})
	}
	return g.Wait()
}

func downloadNationalTreasure(dir string) error {
	slog.Info("Downloading IBGE codes from the National Treasure…")
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
	w := newClient(cnpjToken)
	if err := downloadFederalRevenue(w, ym, dir); err != nil {
		return fmt.Errorf("error downloading CNPJ files: %w", err)
	}
	w = newClient(taxRegimeToken)
	if err := downloadTaxRegime(w, dir); err != nil {
		return fmt.Errorf("error downloading tax regime files: %w", err)
	}
	if err := downloadNationalTreasure(dir); err != nil {
		return fmt.Errorf("error downloading national treasure files: %w", err)
	}
	ls, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range ls {
			if !e.IsDir() {
				if _, err := time.Parse("2006-01", e.Name()); err == nil {
					if e.Name() != ym {
						if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
							slog.Warn("could not remove old year-month file", "file", e.Name(), "error", err)
						}
					}
				}
			}
		}
	}
	_, err = os.Create(filepath.Join(dir, ym))
	return err
}
