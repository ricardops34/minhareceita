package db

const (
	// SQLBatchSize determines the size of the batches used to create the
	// JSON data in SQL-based databases.
	SQLBatchSize = 65536

	// MongoDBBatchSize determines the size of the batches used to create
	// the JSON in MongoDB in the database.
	MongoDBBatchSize = 16384

	companyTableName = "cnpj"
	graphTableName   = "graph"
	metaTableName    = "meta"
)

// GraphEdge represents a connection between two nodes in the graph.
type GraphEdge struct {
	CompanyID   string `json:"cnpj" bson:"company_id"`
	CompanyName string `json:"razao_social" bson:"company_name"`
	PartnerID   string `json:"id" bson:"partner_id"`
	PartnerName string `json:"nome" bson:"partner_name"`
	PartnerCPF  string `json:"cpf,omitempty" bson:"partner_cnpf"`
	PartnerType int    `json:"-" bson:"partner_type"`
}
