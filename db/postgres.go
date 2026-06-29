package db

import (
	"bytes"
	"context"
	"embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"codeberg.org/cuducos/minha-receita/transform"
	"github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"codeberg.org/cuducos/minha-receita/company"
)

const (
	cursorFieldName = "cursor"
	idFieldName     = "id"
	jsonFieldName   = "json"
	keyFieldName    = "key"
	valueFieldName  = "value"
)

//go:embed postgres
var pgsql embed.FS

type sqlTemplate struct {
	path         fs.DirEntry
	embeddedPath string
	key          string
}

func (s *sqlTemplate) render(p *PostgreSQL) (string, error) {
	t, err := template.ParseFS(pgsql, s.embeddedPath)
	if err != nil {
		return "", fmt.Errorf("error parsing %s template: %w", s.path, err)
	}
	var b bytes.Buffer
	if err = t.Execute(&b, p); err != nil {
		return "", fmt.Errorf("error rendering %s template: %w", s.path, err)
	}
	return b.String(), nil

}

func newSQLTemplate(f fs.DirEntry) sqlTemplate {
	return sqlTemplate{
		path:         f,
		embeddedPath: "postgres/" + f.Name(),
		key:          strings.TrimSuffix(f.Name(), filepath.Ext(f.Name())),
	}
}

type ExtraIndex struct {
	IsRoot bool
	Name   string
	Value  string
}

func (e *ExtraIndex) NestedPath() string {
	if e.IsRoot {
		slog.Error("cannot not parse nested path for index at the root of the json", "index", e.Value)
		return ""
	}
	p := strings.SplitN(e.Value, ".", 2)
	if len(p) != 2 {
		slog.Error("could not parse nested path", "index", e.Value)
		return ""
	}
	return fmt.Sprintf("$.%s[*].%s", p[0], p[1])
}

// PostgreSQL database interface.
type PostgreSQL struct {
	pool             *pgxpool.Pool
	uri              string
	schema           string
	getCompanyQuery  string
	metaReadQuery    string
	CompanyTableName string
	MetaTableName    string
	CursorFieldName  string
	IDFieldName      string
	JSONFieldName    string
	KeyFieldName     string
	ValueFieldName   string
	ExtraIndexes     []ExtraIndex
	Logged           bool
}

func (p *PostgreSQL) renderTemplate(key string) (string, error) {
	ls, err := pgsql.ReadDir("postgres")
	if err != nil {
		return "", fmt.Errorf("error looking for templates: %w", err)
	}
	for _, f := range ls {
		s := newSQLTemplate(f)
		if s.key != key {
			continue
		}
		return s.render(p)
	}
	return "", fmt.Errorf("template %s not found", key)
}

// Close closes the PostgreSQL connection
func (p *PostgreSQL) Close() { p.pool.Close() }

// CompanyTableFullName is the name of the schame and table in dot-notation.
func (p *PostgreSQL) CompanyTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.CompanyTableName)
}

// MetaTableFullName is the name of the schame and table in dot-notation.
func (p *PostgreSQL) MetaTableFullName() string {
	return fmt.Sprintf("%s.%s", p.schema, p.MetaTableName)
}

// Create creates the required database table.
func (p *PostgreSQL) Create() error {
	slog.Info("Creating", "table", p.CompanyTableFullName())
	s, err := p.renderTemplate("create")
	if err != nil {
		return fmt.Errorf("error rendering create template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error creating table with: %s\n%w", s, err)
	}
	return nil
}

// Drop drops the database table created by `Create`.
func (p *PostgreSQL) Drop() error {
	slog.Info("Dropping", "table", p.CompanyTableFullName())
	s, err := p.renderTemplate("drop")
	if err != nil {
		return fmt.Errorf("error rendering drop template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error dropping table with: %s\n%w", s, err)
	}
	return nil
}

// CreateCompanies performs a copy to create a batch of companies in the
// database.
func (p *PostgreSQL) CreateCompanies(ctx context.Context, cs []company.Company) error {
	b := make([][]any, len(cs))
	var g errgroup.Group
	for i := range cs {
		g.Go(func() error {
			j, err := cs[i].JSON()
			if err != nil {
				return fmt.Errorf("error serializing company to JSON during import: %w", err)
			}
			b[i] = []any{cs[i].CNPJ, string(j)}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	_, err := p.pool.CopyFrom(
		ctx,
		pgx.Identifier{p.CompanyTableName},
		[]string{idFieldName, jsonFieldName},
		pgx.CopyFromRows(b),
	)
	if err != nil {
		return fmt.Errorf("error while importing data to postgres: %w", err)
	}
	return nil
}

// GetCompany returns the JSON of a company based on a CNPJ number.
func (p *PostgreSQL) GetCompany(ctx context.Context, id string) ([]byte, error) {
	rows, err := p.pool.Query(ctx, p.getCompanyQuery, id)
	if err != nil {
		return nil, fmt.Errorf("error looking for cnpj %s: %w", id, err)
	}
	j, err := pgx.CollectOneRow(rows, pgx.RowTo[[]byte])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("cnpj %s: %w", id, ErrCompanyNotFound)
		}
		return nil, fmt.Errorf("error reading cnpj %s: %w", id, err)
	}
	return j, nil
}

func (p *PostgreSQL) searchQuery(q *Query) *sqlbuilder.SelectBuilder {
	b := sqlbuilder.PostgreSQL.NewSelectBuilder()
	b.Select(p.CursorFieldName, p.JSONFieldName)
	b.From(p.CompanyTableFullName())
	b.OrderByAsc(p.CursorFieldName)
	b.Limit(int(q.Limit))
	if q.Cursor != nil {
		c, err := q.CursorAsInt()
		if err == nil {
			b.Where(b.GreaterThan(p.CursorFieldName, c))
		}
	}
	if len(q.UF) > 0 {
		c := make([]string, len(q.UF))
		for i, v := range q.UF {
			c[i] = fmt.Sprintf(`json -> 'uf' = '"%s"'::jsonb`, v)
		}
		b.Where(b.Or(c...))
	}
	if len(q.Municipio) > 0 {
		c := make([]string, len(q.Municipio)*2)
		for i, v := range q.Municipio {
			c[i] = fmt.Sprintf("json -> 'codigo_municipio' = '%d'::jsonb", v)
			c[i+len(q.Municipio)] = fmt.Sprintf("json -> 'codigo_municipio_ibge' = '%d'::jsonb", v)
		}
		b.Where(b.Or(c...))
	}
	if len(q.NaturezaJuridica) > 0 {
		c := make([]string, len(q.NaturezaJuridica))
		for i, v := range q.NaturezaJuridica {
			c[i] = fmt.Sprintf("json -> 'codigo_natureza_juridica' = '%d'::jsonb", v)
		}
		b.Where(b.Or(c...))
	}
	if len(q.CNAEFiscal) > 0 {
		c := make([]string, len(q.CNAEFiscal))
		for i, v := range q.CNAEFiscal {
			c[i] = fmt.Sprintf("json -> 'cnae_fiscal' = '%d'::jsonb", v)
		}
		b.Where(b.Or(c...))
	}
	if len(q.CNAE) > 0 {
		c := make([]string, len(q.CNAE)+1)
		s := make([]string, len(q.CNAE))
		for i, v := range q.CNAE {
			s[i] = fmt.Sprintf("%d", v)
			c[i] = fmt.Sprintf("json -> 'cnae_fiscal' = '%d'::jsonb", v)
		}
		c[len(q.CNAE)] = fmt.Sprintf(
			"jsonb_path_query_array(json, '$.cnaes_secundarios[*].codigo') @> '[%s]'",
			strings.Join(s, ","),
		)
		b.Where(b.Or(c...))
	}
	if len(q.CNPF) > 0 {
		c := make([]string, len(q.CNPF))
		for i, v := range q.CNPF {
			c[i] = fmt.Sprintf(`jsonb_path_query_array(json, '$.qsa[*].cnpj_cpf_do_socio') @> '["%s"]'`, v)
		}
		b.Where(b.Or(c...))
	}
	return b
}

type postgresRecord struct {
	Cursor  int
	Company []byte
}

// Search returns paginated results with JSON for companies bases on a search
// query
func (p *PostgreSQL) Search(ctx context.Context, q *Query) ([]byte, error) {
	s, a := p.searchQuery(q).Build()
	slog.Debug("paginated search", "query", s, "args", a)
	rows, err := p.pool.Query(ctx, s, a...)
	if err != nil {
		return nil, fmt.Errorf("error searching for %#v: %w", q, err)
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[postgresRecord])
	if err != nil {
		return nil, fmt.Errorf("error reading search result for %#v: %w", q, err)
	}
	var cs [][]byte
	for _, r := range rs {
		cs = append(cs, r.Company)
	}
	var cur string
	if len(rs) == int(q.Limit) {
		cur = fmt.Sprintf("%d", rs[len(rs)-1].Cursor)
	}
	return newPage(cs, cur), nil
}

// PreLoad runs before starting to load data into the database. Currently it
// disables autovacuum on PostgreSQL.
func (p *PostgreSQL) PreLoad() error {
	s, err := p.renderTemplate("pre_load")
	if err != nil {
		return fmt.Errorf("error rendering pre-load template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error during pre load: %s\n%w", s, err)
	}
	return nil
}

// PostLoad runs after loading data into the database. Currently it re-enables
// autovacuum on PostgreSQL.
func (p *PostgreSQL) PostLoad() error {
	s, err := p.renderTemplate("post_load")
	if err != nil {
		return fmt.Errorf("error rendering post-load template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("error during post load: %s\n%w", s, err)
	}
	return nil
}

// MetaSave saves a key/value pair in the metadata table.
func (p *PostgreSQL) MetaSave(k, v string) error {
	if len(k) > 16 {
		return fmt.Errorf("metatable can only take keys that are at maximum 16 chars long")
	}
	s, err := p.renderTemplate("meta_save")
	if err != nil {
		return fmt.Errorf("error rendering meta-save template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s, k, v); err != nil {
		return fmt.Errorf("error saving %s to metadata: %w", k, err)
	}
	return nil
}

// MetaRead reads a key/value pair from the metadata table.
func (p *PostgreSQL) MetaRead(k string) (string, error) {
	rows, err := p.pool.Query(context.Background(), p.metaReadQuery, k)
	if err != nil {
		return "", fmt.Errorf("error looking for metadata key %s: %w", k, err)
	}
	v, err := pgx.CollectOneRow(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("error reading for metadata key %s: %w", k, err)
	}
	return v, nil
}

// CreateExtraIndexes responsible for creating additional indexes in the database
func (p *PostgreSQL) CreateExtraIndexes(idxs []string) error {
	if err := transform.ValidateIndexes(idxs); err != nil {
		return fmt.Errorf("index name error: %w", err)
	}
	for _, idx := range idxs {
		i := ExtraIndex{
			IsRoot: !strings.Contains(idx, "."),
			Name:   fmt.Sprintf("json.%s", idx),
			Value:  idx,
		}
		p.ExtraIndexes = append(p.ExtraIndexes, i)
	}
	s, err := p.renderTemplate("extra_indexes")
	if err != nil {
		return fmt.Errorf("error rendering extra-indexes template: %w", err)
	}
	if _, err := p.pool.Exec(context.Background(), s); err != nil {
		return fmt.Errorf("expected the error to create indexe: %w", err)
	}
	slog.Info(fmt.Sprintf("%d Indexes successfully created in the table %s", len(idxs), p.CompanyTableName))
	return nil
}

type postgresIDRecord struct {
	Cursor int
	ID     string
}

// AllCompanies returns a paginated list of CNPJ numbers from the database.
func (p *PostgreSQL) AllCompanies(ctx context.Context, cursor *string, limit uint32) ([]string, *string, error) {
	b := sqlbuilder.PostgreSQL.NewSelectBuilder().
		Select(p.CursorFieldName, p.IDFieldName).
		From(p.CompanyTableFullName()).
		OrderByAsc(p.CursorFieldName).
		Limit(int(limit))

	if cursor != nil {
		c, err := strconv.Atoi(*cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		b.Where(b.GreaterThan(p.CursorFieldName, c))
	}

	sql, args := b.Build()
	q, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("error listing CNPJs: %w", err)
	}

	rows, err := pgx.CollectRows(q, pgx.RowToStructByPos[postgresIDRecord])
	if err != nil {
		return nil, nil, fmt.Errorf("error reading CNPJs: %w", err)
	}

	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}

	if len(rows) < int(limit) {
		return ids, nil, nil
	}

	cur := fmt.Sprintf("%d", rows[len(rows)-1].Cursor)
	return ids, &cur, nil
}

// NewPostgreSQL creates a new PostgreSQL connection and ping it to make sure it works.
func NewPostgreSQL(a *Args) (PostgreSQL, error) {
	cfg, err := pgxpool.ParseConfig(a.URI)
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("could not create database config: %w", err)
	}
	cfg.MaxConns = int32(a.MaxConns)
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 30 * time.Minute
	conn, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("could not connect to the database: %w", err)
	}
	p := PostgreSQL{
		pool:             conn,
		uri:              a.URI,
		schema:           a.PostgresSchema,
		CompanyTableName: companyTableName,
		MetaTableName:    metaTableName,
		CursorFieldName:  cursorFieldName,
		IDFieldName:      idFieldName,
		JSONFieldName:    jsonFieldName,
		KeyFieldName:     keyFieldName,
		ValueFieldName:   valueFieldName,
		Logged:           a.PostgresLogged,
	}
	p.getCompanyQuery, err = p.renderTemplate("get")
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("error rendering get template: %w", err)
	}
	p.metaReadQuery, err = p.renderTemplate("meta_read")
	if err != nil {
		return PostgreSQL{}, fmt.Errorf("error rendering meta-read template: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.pool.Ping(ctx); err != nil {
		return PostgreSQL{}, fmt.Errorf("could not connect to postgres: %w", err)
	}
	return p, nil
}

// StreamRelationships streams all relationships directly from the main table
// using encoding/csv.
func (p *PostgreSQL) StreamRelationships(ctx context.Context, callback func(Relationship) error) error {
	sql, err := p.renderTemplate("stream_relationships")
	if err != nil {
		return fmt.Errorf("error rendering stream_relationships template: %w", err)
	}

	pool, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("error acquiring connection from pool: %w", err)
	}
	defer pool.Release()

	conn := pool.Conn().PgConn()
	buf, pw := io.Pipe()

	go func() {
		_, err := conn.CopyTo(ctx, pw, sql)
		if err != nil {
			if closeErr := pw.CloseWithError(err); closeErr != nil {
				slog.Warn("could not close pipe writer with error", "error", closeErr)
			}
			return
		}
		if closeErr := pw.Close(); closeErr != nil {
			slog.Warn("could not close pipe writer", "error", closeErr)
		}
	}()

	r := csv.NewReader(buf)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error parsing relationship CSV stream: %w", err)
		}
		if len(rec) < 6 {
			continue
		}
		t, _ := strconv.Atoi(rec[5])
		rel := Relationship{
			CompanyID:   rec[0],
			CompanyName: rec[1],
			PartnerID:   rec[2],
			PartnerName: rec[3],
			PartnerCPF:  rec[4],
			PartnerType: t,
		}
		if err := callback(rel); err != nil {
			return err
		}
	}

	return nil
}

// RelationshipCount returns the count of relationships in the graph (items in qsa lists).
func (p *PostgreSQL) RelationshipCount(ctx context.Context) (int64, error) {
	sql, err := p.renderTemplate("relationship_count")
	if err != nil {
		return 0, fmt.Errorf("error rendering relationship_count template: %w", err)
	}

	var n int64
	err = p.pool.QueryRow(ctx, sql).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("error querying relationship count: %w", err)
	}
	return n, nil
}
