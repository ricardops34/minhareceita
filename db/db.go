package db

const (
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
