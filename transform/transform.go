package transform

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codeberg.org/cuducos/minha-receita/company"
	"codeberg.org/cuducos/minha-receita/graph"
	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

var validIndexName = regexp.MustCompile(`^[a-z_.]+$`)

// ValidateIndexes checks that the given index names are valid JSON field paths
// (only lowercase letters, underscores, and dots).
func ValidateIndexes(idxs []string) error {
	for _, idx := range idxs {
		if !validIndexName.MatchString(idx) {
			return fmt.Errorf("invalid index name %q: only lowercase letters, underscores and dots are allowed", idx)
		}
	}
	return nil
}

var extraIndexes = [...]string{
	"cnae_fiscal",
	"cnaes_secundarios.codigo",
	"codigo_municipio",
	"codigo_municipio_ibge",
	"codigo_natureza_juridica",
	"qsa.cnpj_cpf_do_socio",
	"uf",
}

// In Oct. 2025 the Federal Revenue started using the country code 367. which is
// not present in Paises.zip. The issue was officially reported to them via
// Fala.BR. They replied but did not seem to care about updating the dataset.
//
// It seems safe to assume this is England:
// 1. Other official documents from the institution uses 367 for England, eg.:
// https://balanca.economia.gov.br/balanca/bd/tabelas/PAIS.csv or
// https://www.cenofisco.com.br/arquivos/BDFlash/IR_IN_RFB_1076.pdf
// 2. Paises.zip contains a CSV ordered by country name and “Inglaterra” would
// match this ordering
//
// The same logic was used to other unmatched country codes:
var extraCountries = map[int]string{
	15:  "Aland, Ilhas",
	150: "Canal, Ilhas do (Guernsey)",
	151: "Canárias, Ilhas",
	200: "Curaçao",
	321: "Guernsey",
	359: "Ilha de Man",
	367: "Inglaterra",
	393: "Jersey",
	449: "Macedônia",
	452: "Madeira, Ilha da",
	498: "Montenegro",
	578: "Palestina",
	678: "Saint Kitts e Nevis",
	699: "Sint Maarten",
	737: "Sérvia",
	994: "A Designar",
}

type database interface {
	PreLoad() error
	CreateCompanies(context.Context, []company.Company) error
	PostLoad() error
	CreateExtraIndexes([]string) error
	MetaSave(string, string) error
}

type graphWriter interface {
	Save(*slog.Logger, *company.Relationship) error
	Close() error
	Path() string
}

func sources() map[string]*source { // all but Estabelecimentos (this one is loaded later on)
	srcs := []*source{
		newCompanySrc("Cnaes", ';', false, false),
		newCompanySrc("Empresas", ';', false, false),
		newTaxSrc("Imunes e Isentas", "entidades-imunes-e-isentas", ',', true, true),
		newTaxSrc("Lucro Arbitrado", "entidades-lucro-arbitrado", ',', true, true),
		newTaxSrc("Lucro Presumido", "entidades-lucro-presumido", ',', true, true),
		newTaxSrc("Lucro Real", "entidades-lucro-real", ',', true, true),
		newCompanySrc("Motivos", ';', false, false),
		newCompanySrc("Municipios", ';', false, false),
		newCompanySrc("Naturezas", ';', false, false),
		newCompanySrc("Paises", ';', false, false),
		newCompanySrc("Qualificacoes", ';', false, false),
		newCompanySrc("Simples", ';', false, false),
		newCompanySrc("Socios", ';', false, true),
		newIBGESrc("tabmun", ';', false, false),
	}
	m := make(map[string]*source)
	for _, src := range srcs {
		m[src.key] = src
	}
	return m
}

func newProgressBar(label string, srcs int) (*progressbar.ProgressBar, error) {
	bar := progressbar.NewOptions(
		srcs, // it has a bug starting At zero, so we compensate for it later
		progressbar.OptionFullWidth(),
		progressbar.OptionSetDescription(label),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionOnCompletion(func() { fmt.Println() }),
	)
	return bar, bar.RenderBlank()
}

func saveUpdatedAt(db database, date string) error {
	slog.Info("Saving the updated at date to the database…")
	return db.MetaSave("updated-at", date)
}

func postLoad(db database, wait <-chan struct{}) error {
	logs := make(chan string, 4) // so we have room for the 4 logs below
	go func() {
		if wait != nil {
			<-wait
		}
		for msg := range logs {
			slog.Info(msg)
		}
	}()
	logs <- "Consolidating the database…"
	if err := db.PostLoad(); err != nil {
		return err
	}
	logs <- "Database consolidated!"
	logs <- "Creating indexes…"
	if err := db.CreateExtraIndexes(extraIndexes[:]); err != nil {
		return err
	}
	logs <- "Indexes created!"
	close(logs)
	return nil
}

func findUpdatedAt(dir string) (string, error) {
	ls, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read directory %s: %w", dir, err)
	}
	var ym string
	for _, e := range ls {
		if e.IsDir() {
			continue
		}
		if _, err := time.Parse("2006-01", e.Name()); err == nil {
			ym = e.Name()
		}
	}
	if ym != "" {
		return ym, nil
	}
	return "", fmt.Errorf("could not find YYYY-MM file in %s", dir)
}

func Transform(dir string, db database, gw graphWriter, batch int, privacy bool) error {
	if db != nil {
		if err := db.PreLoad(); err != nil {
			return err
		}
	}
	srcs := sources()
	u, err := findUpdatedAt(dir)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", fmt.Sprintf("minha-receita-%s-*", time.Now().Format("20060102150405")))
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmp); err != nil {
			slog.Warn("could not remove temporary directory", "path", tmp, "error", err)
		}
	}()
	kv, err := newBadger(tmp, false)
	if err != nil {
		return fmt.Errorf("could not create badger database: %w", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			slog.Warn("could not close badger database", "error", err)
		}
	}()
	bar, err := newProgressBar("[1/3] Loading data to key-value storage", len(srcs))
	if err != nil {
		return fmt.Errorf("could not create a progress bar: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var g errgroup.Group
	for _, src := range srcs {
		g.Go(func() error {
			switch src.kind {
			case CompanySrc:
				return loadCSVs(ctx, dir, src, bar, kv)
			case TaxSrc:
				return loadCSVs(ctx, dir, src, bar, kv)
			case IBGESrc:
				return loadCSVs(ctx, dir, src, bar, kv)
			}
			return fmt.Errorf("unknown source kind %d for %s", src.kind, src.prefix)
		})
	}
	for k, v := range extraCountries {
		g.Go(func() error {
			return kv.put(srcs["pai"], fmt.Sprintf("%d", k), []string{v})
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if err := kv.flush(); err != nil {
		return fmt.Errorf("could not flush key-value storage: %w", err)
	}
	src := newCompanySrc("Estabelecimentos", ';', false, false)
	w, err := newWriter(db, gw, kv, srcs, batch, privacy, dir, src)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.write(ctx); err != nil {
		return err
	}
	if gw != nil {
		if err := gw.Close(); err != nil {
			return err
		}
	}
	g = errgroup.Group{}
	wait := make(chan struct{}) // avoid breaking the compress graph progress bar
	if db != nil {
		g.Go(func() error { return postLoad(db, wait) })
	}
	if gw != nil {
		g.Go(func() error {
			defer close(wait)
			b := progressbar.NewOptions64(
				-1,
				progressbar.OptionSetDescription("[3/3] Compressing graph directory"),
				progressbar.OptionShowBytes(true),
				progressbar.OptionShowCount(),
				progressbar.OptionShowElapsedTimeOnFinish(),
				progressbar.OptionFullWidth(),
				progressbar.OptionUseANSICodes(true),
				progressbar.OptionOnCompletion(func() { fmt.Println() }),
			)
			return graph.Compress(gw.Path(), b)
		})
	} else {
		close(wait)
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if db != nil {
		return saveUpdatedAt(db, u)
	}
	return nil
}

func Cleanup() error {
	return filepath.WalkDir(os.TempDir(), func(pth string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "minha-receita-") {
			return nil
		}
		part := strings.Split(d.Name(), "-")
		if len(part) != 4 {
			return nil
		}
		if _, err := time.Parse("20060102150405", part[2]); err != nil {
			return nil
		}
		fmt.Printf("Removing %s\n", pth)
		if err := os.RemoveAll(pth); err != nil {
			return err
		}
		return fs.SkipDir
	})
}
