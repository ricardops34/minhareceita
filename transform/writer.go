package transform

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"codeberg.org/cuducos/minha-receita/company"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
	"golang.org/x/text/encoding/charmap"
)

type writer struct {
	db    database
	graph graphWriter
	kv    *kv
	batch int

	dir     string
	src     *source
	srcs    map[string]*source
	privacy bool

	bar     *progressbar.ProgressBar
	log     *slog.Logger
	logFile *os.File
	logPath string

	once sync.Once
	seen *seenDB
}

func (w *writer) write(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	ws := max(1, runtime.NumCPU())
	ch := make(chan []company.Company, ws)

	for range ws {
		g.Go(func() error {
			for b := range ch {
				if err := w.saveBatch(ctx, b); err != nil {
					return err
				}
			}
			return nil
		})
	}

	ls, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", w.dir, err)
	}

	g.Go(func() error {
		defer close(ch)
		var ps errgroup.Group
		ps.SetLimit(max(1, runtime.NumCPU()))
		for _, pth := range ls {
			if !strings.HasPrefix(pth.Name(), w.src.prefix) || strings.ToLower(filepath.Ext(pth.Name())) != ".zip" {
				continue
			}
			p := filepath.Join(w.dir, pth.Name())
			ps.Go(func() error {
				return w.processBatches(ctx, p, ch)
			})
		}
		return ps.Wait()
	})

	return g.Wait()
}

func (w *writer) stream(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)
	ws := max(1, runtime.NumCPU())

	var ch chan company.Company
	if w.db != nil {
		ch = make(chan company.Company, ws)
		g.Go(func() error {
			return w.db.StreamCompanies(ctx, ch)
		})
	}

	var rel chan *company.Relationship
	if w.graph != nil {
		rel = make(chan *company.Relationship, ws)
		g.Go(func() error {
			s, sctx := errgroup.WithContext(ctx)
			s.SetLimit(max(16, runtime.NumCPU())) // 16 is Badger internal limit
			for r := range rel {
				s.Go(func() error {
					select {
					case <-sctx.Done():
						return sctx.Err()
					default:
						return w.graph.Save(w.log, r)
					}
				})
			}
			return s.Wait()
		})
	}

	ls, err := os.ReadDir(w.dir)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", w.dir, err)
	}

	g.Go(func() error {
		if ch != nil {
			defer close(ch)
		}
		if rel != nil {
			defer close(rel)
		}
		var ps errgroup.Group
		ps.SetLimit(max(1, runtime.NumCPU()))
		for _, pth := range ls {
			if !strings.HasPrefix(pth.Name(), w.src.prefix) || strings.ToLower(filepath.Ext(pth.Name())) != ".zip" {
				continue
			}
			p := filepath.Join(w.dir, pth.Name())
			ps.Go(func() error {
				return w.processStream(ctx, p, ch, rel)
			})
		}
		return ps.Wait()
	})

	return g.Wait()
}

func (w *writer) processBatches(ctx context.Context, pth string, ch chan<- []company.Company) error {
	a, err := zip.OpenReader(pth)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", pth, err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			slog.Warn("could not close archive", "path", pth, "error", err)
		}
	}()
	for _, f := range a.File {
		w.bar.AddMax64(int64(f.UncompressedSize64))
		w.once.Do(func() {
			w.bar.AddMax(-1) // compensate for the extra byte added when creating the bar
		})
	}
	var g errgroup.Group
	g.SetLimit(max(1, runtime.NumCPU()))
	for _, f := range a.File {
		if f.FileInfo().IsDir() {
			continue
		}
		g.Go(func() error {
			return w.batchCSV(ctx, f, ch)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func (w *writer) processStream(ctx context.Context, pth string, ch chan<- company.Company, rel chan<- *company.Relationship) error {
	a, err := zip.OpenReader(pth)
	if err != nil {
		return fmt.Errorf("could not open %s: %w", pth, err)
	}
	defer func() {
		if err := a.Close(); err != nil {
			slog.Warn("could not close archive", "path", pth, "error", err)
		}
	}()
	for _, f := range a.File {
		w.bar.AddMax64(int64(f.UncompressedSize64))
		w.once.Do(func() {
			w.bar.AddMax(-1)
		})
	}
	var g errgroup.Group
	g.SetLimit(max(1, runtime.NumCPU()))
	for _, f := range a.File {
		if f.FileInfo().IsDir() {
			continue
		}
		g.Go(func() error {
			return w.streamCSV(ctx, f, ch, rel)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

func (w *writer) saveBatch(ctx context.Context, b []company.Company) error {
	if len(b) == 0 {
		return nil
	}
	g, ctx := errgroup.WithContext(ctx)
	if w.db != nil {
		g.Go(func() error {
			return w.db.CreateCompanies(ctx, b)
		})
	}
	if w.graph != nil {
		ch := make(chan *company.Relationship)
		g.Go(func() error {
			defer close(ch)
			for _, c := range b {
				c.Relationships(ch)
			}
			return nil
		})
		g.Go(func() error {
			s, ctx := errgroup.WithContext(ctx)
			s.SetLimit(max(16, runtime.NumCPU())) // 16 is Badger internal limit
			for r := range ch {
				s.Go(func() error {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						return w.graph.Save(w.log, r)
					}
				})
			}
			return s.Wait()
		})
	}
	return g.Wait()
}

func (w *writer) batchCSV(ctx context.Context, f *zip.File, ch chan<- []company.Company) error {
	r, err := f.Open()
	if err != nil {
		return fmt.Errorf("could not read %s: %w", f.Name, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("Could not close csv reader", "name", f.Name, "error", err)
		}
	}()
	cr := countReader{r, 0}
	csvr := csv.NewReader(charmap.ISO8859_15.NewDecoder().Reader(&cr))
	csvr.Comma = w.src.sep
	b := make([]company.Company, 0, w.batch)
	var prev int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			row, err := csvr.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					if len(b) > 0 {
						select {
						case <-ctx.Done():
							return ctx.Err()
						case ch <- b:
						}
					}
					return nil
				}
				return fmt.Errorf("error reading %s: %w", f.Name, err)
			}
			if len(row) < 2 {
				return fmt.Errorf("unexpected row with %d columns in %s", len(row), w.src.prefix)
			}
			for n := range row {
				row[n] = cleanupColumn(row[n])
			}
			c, err := newCompany(w.log, w.srcs, w.kv, row)
			if err != nil {
				return fmt.Errorf("could not create company %v: %w", row[:3], err)
			}
			if w.privacy {
				c.WithPrivacy()
			}

			ok, err := w.seen.check(c.CNPJ)
			if err != nil {
				return err
			}
			if ok {
				w.log.Warn("Skipping duplicate CNPJ", "cnpj", c.CNPJ)
				continue
			}

			b = append(b, *c)
			if len(b) >= w.batch {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ch <- b:
				}
				b = make([]company.Company, 0, w.batch)
			}
			s := cr.read - prev
			if s > 0 {
				if err := w.bar.Add64(s); err != nil {
					slog.Warn("could not update the progress bar", "error", err)
				}
			}
			prev = cr.read
		}
	}
}

func (w *writer) streamCSV(ctx context.Context, f *zip.File, ch chan<- company.Company, rel chan<- *company.Relationship) error {
	r, err := f.Open()
	if err != nil {
		return fmt.Errorf("could not read %s: %w", f.Name, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			slog.Warn("Could not close csv reader", "name", f.Name, "error", err)
		}
	}()
	cr := countReader{r, 0}
	csvr := csv.NewReader(charmap.ISO8859_15.NewDecoder().Reader(&cr))
	csvr.Comma = w.src.sep
	var prev int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			row, err := csvr.Read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("error reading %s: %w", f.Name, err)
			}
			if len(row) < 2 {
				return fmt.Errorf("unexpected row with %d columns in %s", len(row), w.src.prefix)
			}
			for n := range row {
				row[n] = cleanupColumn(row[n])
			}
			c, err := newCompany(w.log, w.srcs, w.kv, row)
			if err != nil {
				return fmt.Errorf("could not create company %v: %w", row[:3], err)
			}
			if w.privacy {
				c.WithPrivacy()
			}

			ok, err := w.seen.check(c.CNPJ)
			if err != nil {
				return err
			}
			if ok {
				w.log.Warn("Skipping duplicate CNPJ", "cnpj", c.CNPJ)
				continue
			}

			if rel != nil {
				c.Relationships(rel)
			}

			if ch != nil {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ch <- *c:
				}
			}

			s := cr.read - prev
			if s > 0 {
				if err := w.bar.Add64(s); err != nil {
					slog.Warn("could not update the progress bar", "error", err)
				}
			}
			prev = cr.read
		}
	}
}

func (w *writer) Close() {
	if w.logFile != nil {
		if err := w.logFile.Close(); err != nil {
			slog.Warn("could not close the log file", "name", w.logPath, "error", err)
		}
		i, err := os.Stat(w.logPath)
		if err != nil {
			slog.Warn("could not check log file size", "name", w.logPath, "error", err)
			return
		}
		if i.Size() == 0 {
			if err := os.Remove(w.logPath); err != nil {
				slog.Warn("could not delete empty log file", "name", w.logPath, "error", err)
			}
		}
	}
}

func newWriter(db database, graph graphWriter, kv *kv, seen *seenDB, srcs map[string]*source, batch int, privacy bool, ext string, src *source) (*writer, error) {
	bar, err := newProgressBar("[2/3] Writing JSONs", 1)
	if err != nil {
		return nil, fmt.Errorf("could not create a progress bar: %w", err)
	}
	n := fmt.Sprintf("minha-receita-etl-%s.log", time.Now().Format("20060102150405"))
	f, err := os.Create(n)
	if err != nil {
		return nil, fmt.Errorf("could not create log file %s: %w", n, err)
	}
	log := slog.New(slog.NewJSONHandler(f, nil))
	return &writer{
		db:      db,
		graph:   graph,
		kv:      kv,
		srcs:    srcs,
		batch:   batch,
		privacy: privacy,
		bar:     bar,
		log:     log,
		logFile: f,
		logPath: n,
		src:     src,
		dir:     ext,
		seen:    seen,
	}, nil
}
