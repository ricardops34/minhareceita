package db

import "os"

const maxConnsDefault = 16

// Args holds the configuration for connecting to and using a database.
type Args struct {
	URI            string
	MaxConns       int

	PostgresSchema string
	PostgresLogged bool
}

// NewArgs creates an Args from environment variables.
func NewArgs() Args {
	return Args{
		URI:      os.Getenv("DATABASE_URL"),
		MaxConns: maxConnsDefault,
	}
}

// SetURI sets the database URI, ignoring empty values so a CLI flag
// can override an env-var-backed default without stomping it.
func (a *Args) SetURI(u string) {
	if u != "" {
		a.URI = u
	}
}
