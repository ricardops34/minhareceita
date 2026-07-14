package cmd

import (
	"context"
	"fmt"
	"strings"

	"codeberg.org/cuducos/minha-receita/company"
	"codeberg.org/cuducos/minha-receita/db"
)

type database interface {
	Create() error
	Drop() error
	Close()

	// transform
	PreLoad() error
	CreateCompanies(context.Context, []company.Company) error
	StreamCompanies(context.Context, <-chan company.Company) error
	PostLoad() error
	MetaSave(string, string) error

	// extra indexes
	CreateExtraIndexes(idxs []string) error

	// api
	GetCompany(context.Context, string) ([]byte, error)
	Search(context.Context, *db.Query) ([]byte, error)
	MetaRead(string) (string, error)
	AllCompanies(context.Context, *string, uint32) ([]string, *string, error)
}

func loadDatabase(args *db.Args) (database, error) {
	if args.URI == "" {
		return nil, fmt.Errorf("could not find a database URI, set the DATABASE_URL environment variable with the credentials for a database")
	}
	if strings.HasPrefix(args.URI, "postgres://") || strings.HasPrefix(args.URI, "postgresql://") {
		d, err := db.NewPostgreSQL(args)
		return &d, err
	}
	if args.PostgresLogged {
		return nil, fmt.Errorf("the --logged flag is only available for PostgreSQL databases")
	}
	if strings.HasPrefix(args.URI, "mongodb://") {
		d, err := db.NewMongoDB(args)
		return &d, err
	}
	return nil, fmt.Errorf("database uri does not seem to be a valid Postgres or MongoDB URI")
}
