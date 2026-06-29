package db

import "errors"

var ErrCompanyNotFound = errors.New("company not found")

const (
	// SQLBatchSize determines the size of the batches used to create the
	// JSON data in SQL-based databases.
	SQLBatchSize = 65536

	// MongoDBBatchSize determines the size of the batches used to create
	// the JSON in MongoDB in the database.
	MongoDBBatchSize = 16384

	companyTableName = "cnpj"
	metaTableName    = "meta"
)
