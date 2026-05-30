package db

const (
	companyTableName = "cnpj"
	graphTableName   = "graph"
	metaTableName    = "meta"
)

type graphPartner struct {
	PartnerID string `json:"partner_id"`
	Name      string `json:"name,omitempty"`
	CPF       string `json:"cpf,omitempty"`
}

type companyPartnersResponse struct {
	CompanyID string         `json:"company_id"`
	Name      string         `json:"name"`
	Partners  []graphPartner `json:"partners"`
}

type graphCompany struct {
	CNPJ string `json:"cnpj"`
	Name string `json:"name"`
}

type partnerCompaniesResponse struct {
	PartnerID string         `json:"partner_id"`
	Name      string         `json:"name,omitempty"`
	CPF       string         `json:"cpf,omitempty"`
	Companies []graphCompany `json:"companies"`
}

type companyPartnerRecord struct {
	CompanyName string `bson:"company_name"`
	PartnerID   string `bson:"partner_id"`
	PartnerName string `bson:"partner_name"`
	PartnerCNPF string `bson:"partner_cnpf"`
	PartnerType int    `bson:"partner_type"`
}

type partnerCompanyRecord struct {
	PartnerName string `bson:"partner_name"`
	PartnerCNPF string `bson:"partner_cnpf"`
	CompanyID   string `bson:"company_id"`
	CompanyName string `bson:"company_name"`
	PartnerType int    `bson:"partner_type"`
}
