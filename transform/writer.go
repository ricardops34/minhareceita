package transform

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/text/encoding/charmap"
)

func worker(ctx context.Context, db database, s int, ch <-chan []string) error {
	var b [][]string
	for {
		select {
		case <-ctx.Done():
			return nil
		case row, ok := <-ch:
			if !ok {
				if len(b) > 0 {
					return db.CreateCompanies(b)
				}
				return nil
			}
			b = append(b, row)
			if len(b) >= s {
				if err := db.CreateCompanies(b); err != nil {
					return err
				}
				b = [][]string{}
			}
		}
	}
}

func writeJSONs(ctx context.Context, srcs map[string]*source, kv *kv, db database, maxDB, batch int, ext string, privacy bool) error {
	bar, err := newProgressBar("[3/3] Writing JSONs", 1)
	if err != nil {
		return fmt.Errorf("could not create a progress bar: %w", err)
	}
	defer func() {
		bar.AddMax(-1) // compensate for the extra byte added when creating the bar
	}()
	n := fmt.Sprintf("minha-receita-etl-%s.log", time.Now().Format("20060102150405"))
	f, err := os.Create(n)
	if err != nil {
		return fmt.Errorf("could not create log file %s: %w", n, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close the log file", "name", n, "error", err)
		}
		i, err := os.Stat(n)
		if err != nil {
			slog.Warn("could not check log file size", "name", n, "error", err)
			return
		}
		if i.Size() != 0 {
			return
		}
		if err := os.Remove(n); err != nil {
			slog.Warn("could not delete empty log file", "name", n, "error", err)
		}
	}()
	log := slog.New(slog.NewJSONHandler(f, nil))
	src := newCompanySrc("Estabelecimentos", ';', false, false)
	buf := &sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
	ch := make(chan []string, batch*maxDB)
	var consumers errgroup.Group
	for range maxDB {
		consumers.Go(func() error {
			return worker(ctx, db, batch, ch)
		})
	}
	pths, err := os.ReadDir(ext)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", ext, err)
	}
	var producers errgroup.Group
	for _, pth := range pths {
		if !strings.HasPrefix(pth.Name(), src.prefix) || filepath.Ext(pth.Name()) != ".zip" {
			continue
		}
		p := pth
		producers.Go(func() error {
			pth := filepath.Join(ext, p.Name())
			a, err := zip.OpenReader(pth)
			if err != nil {
				return fmt.Errorf("could not open %s: %w", pth, err)
			}
			defer func() {
				if err := a.Close(); err != nil {
					slog.Warn("could not close archive", "path", pth, "error", err)
				}
			}()
			for _, z := range a.File {
				bar.AddMax64(int64(z.UncompressedSize64))
			}
			var g errgroup.Group
			for _, z := range a.File {
				if z.FileInfo().IsDir() {
					continue
				}
				g.Go(func() error {
					f, err := z.Open()
					if err != nil {
						return fmt.Errorf("could not read %s from %s: %w", z.Name, pth, err)
					}
					defer func() {
						if err := f.Close(); err != nil {
							slog.Warn("Could not close csv reader", "path", pth, "name", z.Name, "error", err)
						}
					}()
					b := countReader{f, 0}
					r := csv.NewReader(charmap.ISO8859_15.NewDecoder().Reader(&b))
					r.Comma = src.sep
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
								return fmt.Errorf("error reading %s: %w", pth, err)
							}
							if len(row) < 2 {
								return fmt.Errorf("unexpected row with %d columns in %s", len(row), src.prefix)
							}
							for n := range row {
								row[n] = cleanupColumn(row[n])
							}
							c, err := newCompany(log, srcs, kv, row)
							if err != nil {
								return fmt.Errorf("could not create company %v: %w", row[:3], err)
							}
							if privacy {
								c.withPrivacy()
							}
							j, err := c.JSON(buf)
							if err != nil {
								return err
							}
							ch <- []string{c.CNPJ, j}
							s := b.read - prev
							if s > 0 {
								if err := bar.Add64(s); err != nil {
									slog.Warn("could not update the progress bar", "error", err)
								}
							}
							prev = b.read
						}
					}
				})
			}
			if err := g.Wait(); err != nil {
				return err
			}
			if e := os.Remove(pth); e != nil {
				slog.Warn("could not remove", "path", pth, "error", e)
			}
			return nil
		})
	}
	err1 := producers.Wait()
	close(ch)
	err2 := consumers.Wait()
	if err1 != nil && err2 != nil {
		return fmt.Errorf("errors writing json: (producer error) %w, (consumer error) %w", err1, err2)
	}
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}
