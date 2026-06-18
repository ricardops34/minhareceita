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

	"github.com/schollz/progressbar/v3"
)

type writer struct {
	db      database
	kv      *kv
	srcs    map[string]*source
	batch   int
	privacy bool
	buf     *sync.Pool
	bar     *progressbar.ProgressBar
	log     *slog.Logger
	logFile *os.File
	logPath string
	src     *source
	ext     string
	once    sync.Once
}

func (w *writer) write(ctx context.Context) error {
	pths, err := os.ReadDir(w.ext)
	if err != nil {
		return fmt.Errorf("could not read directory %s: %w", w.ext, err)
	}
	var g errgroup.Group
	for _, pth := range pths {
		if !strings.HasPrefix(pth.Name(), w.src.prefix) || filepath.Ext(pth.Name()) != ".zip" {
			continue
		}
		p := filepath.Join(w.ext, pth.Name())
		g.Go(func() error {
			return w.process(ctx, p)
		})
	}
	return g.Wait()
}

func (w *writer) process(ctx context.Context, pth string) error {
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
	for _, f := range a.File {
		if f.FileInfo().IsDir() {
			continue
		}
		g.Go(func() error {
			return w.processCSV(ctx, f)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if e := os.Remove(pth); e != nil {
		slog.Warn("could not remove", "path", pth, "error", e)
	}
	return nil
}

func (w *writer) processCSV(ctx context.Context, f *zip.File) error {
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
	b := make([][]string, 0, w.batch)
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
						return w.db.CreateCompanies(ctx, b)
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
				c.withPrivacy()
			}
			j, err := c.JSON(w.buf)
			if err != nil {
				return err
			}
			b = append(b, []string{c.CNPJ, j})
			if len(b) >= w.batch {
				if err := w.db.CreateCompanies(ctx, b); err != nil {
					return err
				}
				b = b[:0]
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

func newWriter(db database, kv *kv, srcs map[string]*source, batch int, privacy bool, ext string, src *source) (*writer, error) {
	bar, err := newProgressBar("[3/3] Writing JSONs", 1)
	if err != nil {
		return nil, fmt.Errorf("could not create a progress bar: %w", err)
	}
	n := fmt.Sprintf("minha-receita-etl-%s.log", time.Now().Format("20060102150405"))
	f, err := os.Create(n)
	if err != nil {
		return nil, fmt.Errorf("could not create log file %s: %w", n, err)
	}
	log := slog.New(slog.NewJSONHandler(f, nil))
	buf := &sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
	return &writer{
		db:      db,
		kv:      kv,
		srcs:    srcs,
		batch:   batch,
		privacy: privacy,
		buf:     buf,
		bar:     bar,
		log:     log,
		logFile: f,
		logPath: n,
		src:     src,
		ext:     ext,
	}, nil
}
