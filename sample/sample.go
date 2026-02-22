package sample

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

const (
	// DefaultMaxLines to use when creating sample data
	DefaultMaxLines = 10000

	// DefaultTargetDir to use when creating sample data
	DefaultTargetDir = "sample"
)

func sampleLines(r io.Reader, w io.Writer, m int) error {
	var c int
	s := bufio.NewScanner(r)
	for s.Scan() {
		c++
		if c > m {
			break
		}
		t := s.Text() + "\n"
		_, err := w.Write([]byte(t))
		if err != nil {
			return fmt.Errorf("error writing sample: %w", err)
		}
	}
	if err := s.Err(); err != nil {
		return fmt.Errorf("error reading lines: %w", err)
	}
	return nil
}

func makeSampleFromCSV(src, outDir string, m int) (err error) { // using named return so we can set it in the defer call
	name := filepath.Base(src)
	out := filepath.Join(outDir, name)

	r, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", src, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("could not close", "path", src, "error", err)
		}
	}()

	w, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", out, err)
	}
	defer func() {
		if e := w.Close(); e != nil && err == nil {
			err = fmt.Errorf("error closing %s: %w", out, e)
		}
	}()

	if err := sampleLines(r, w, m); err != nil {
		return fmt.Errorf("error creating sample %s from %s: %w", out, src, err)
	}

	return nil
}

// sampleInnerZip reads a zip from r, samples the CSVs inside it, and writes
// a new zip with the sampled content to w. Used for the bundled zip case
// where the outer zip (e.g. YYYY-MM.zip) contains inner zip files.
func sampleInnerZip(r io.Reader, w io.Writer, m int) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("error reading inner zip: %w", err)
	}
	inner, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("error parsing inner zip: %w", err)
	}
	zw := zip.NewWriter(w)
	for _, z := range inner.File {
		if z.FileInfo().IsDir() {
			continue
		}
		fSrc, err := z.Open()
		if err != nil {
			return fmt.Errorf("error opening %s in inner zip: %w", z.Name, err)
		}
		defer func() {
			if err := fSrc.Close(); err != nil {
				slog.Warn("could not close", "path", z.Name, "error", err)
			}
		}()
		fOut, err := zw.Create(z.Name)
		if err != nil {
			return fmt.Errorf("error creating %s in sampled inner zip: %w", z.Name, err)
		}
		if err := sampleLines(fSrc, fOut, m); err != nil {
			return fmt.Errorf("error sampling %s: %w", z.Name, err)
		}
	}
	return zw.Close()
}

func makeSampleFromZIP(src, outDir string, m int) (err error) { // using named return so we can set it in the defer call
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("error opening %s: %w", src, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("could not close", "path", src, "error", err)
		}
	}()

	name := filepath.Base(src)
	out := filepath.Join(outDir, name)

	o, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("error creating %s: %w", out, err)
	}
	defer func() {
		if e := o.Close(); e != nil && err == nil {
			err = fmt.Errorf("error closing %s: %w", out, e)
		}
	}()

	buf := bufio.NewWriter(o)
	zw := zip.NewWriter(buf)
	defer func() {
		if e := zw.Close(); e != nil && err == nil {
			err = fmt.Errorf("could not write zip buffer: %w", e)
		}
	}()

	for _, z := range r.File {
		if z.FileInfo().IsDir() {
			continue
		}
		fSrc, err := z.Open()
		if err != nil {
			return fmt.Errorf("error reading %s in %s: %w", z.Name, src, err)
		}
		defer func() {
			if err := fSrc.Close(); err != nil {
				slog.Warn("could not close", "path", z.Name, "error", err)
			}
		}()
		if strings.ToLower(filepath.Ext(z.Name)) == ".zip" {
			// Bundled zip: outer zip contains inner zip files (e.g. YYYY-MM.zip)
			fOut, err := zw.Create(z.Name)
			if err != nil {
				return fmt.Errorf("error creating %s in output: %w", z.Name, err)
			}
			if err := sampleInnerZip(fSrc, fOut, m); err != nil {
				return fmt.Errorf("error sampling inner zip %s: %w", z.Name, err)
			}
		} else {
			// Regular zip: outer zip contains a CSV (e.g. entidades-*.zip)
			base := strings.TrimSuffix(name, filepath.Ext(name))
			fOut, err := zw.Create(base)
			if err != nil {
				return fmt.Errorf("error creating %s in output: %w", base, err)
			}
			if err := sampleLines(fSrc, fOut, m); err != nil {
				return fmt.Errorf("error sampling %s in %s: %w", z.Name, src, err)
			}
			break // only one CSV per regular zip
		}
	}
	return nil
}

func makeSample(src, outDir string, m int) error {
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".zip":
		return makeSampleFromZIP(src, outDir, m)
	case ".csv":
		return makeSampleFromCSV(src, outDir, m)
	}
	return fmt.Errorf("no make sample handler for %s", ext)
}

// Sample generates sample data on the target directory, coping the first `m`
// lines of each file from the source directory.
func Sample(src, target string, m int) error {
	if src == target {
		return fmt.Errorf("data directory and target directory cannot be the same")
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("error creating directory %s: %w", target, err)
	}
	ls, err := filepath.Glob(filepath.Join(src, "*.zip"))
	if err != nil {
		return fmt.Errorf("error looking for zip files in %s: %w", target, err)
	}
	if len(ls) == 0 {
		return errors.New("source directory %s has no zip files")
	}
	bar := progressbar.Default(int64(len(ls)))
	defer func() {
		if err := bar.Close(); err != nil {
			slog.Warn("could not close the progress bar", "error", err)
		}
	}()
	bar.Describe("Creating sample files")
	if err := bar.RenderBlank(); err != nil {
		return fmt.Errorf("error rendering the progress bar: %w", err)
	}
	var g errgroup.Group
	for _, pth := range ls {
		g.Go(func() error {
			if err := makeSample(pth, target, m); err != nil {
				return err
			}
			return bar.Add(1)
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("error creating samples: %w", err)
	}
	return nil
}
