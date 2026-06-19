package company

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

const dateOutputFormat = "2006-01-02"

// maskCPF masks a CPF number from company's name tail for privacy.
func maskCPF(name string) string {
	if len(name) < 11 {
		return name
	}
	t := name[len(name)-11:]
	for _, c := range t {
		if c < '0' || c > '9' {
			return name
		}
	}
	if len(name) > 11 {
		prev := name[len(name)-12]
		if prev >= '0' && prev <= '9' {
			return name
		}
	}
	return name[:len(name)-11] + "***" + t[3:8] + "***"
}

type Date time.Time

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	t, err := time.Parse(dateOutputFormat, s)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

func (d *Date) MarshalJSON() ([]byte, error) {
	t := time.Time(*d)
	return []byte(`"` + t.Format(dateOutputFormat) + `"`), nil
}

func (d Date) MarshalBSONValue() (byte, []byte, error) {
	t := time.Time(d)
	return byte(bson.TypeString), bsoncore.AppendString(nil, t.Format(dateOutputFormat)), nil
}

func (d *Date) UnmarshalBSONValue(t byte, v []byte) error {
	switch t {
	case byte(bson.TypeString):
		s, _, ok := bsoncore.ReadString(v)
		if !ok {
			return fmt.Errorf("invalid bson string")
		}
		if s == "" {
			return nil
		}
		p, err := time.Parse(dateOutputFormat, s)
		if err != nil {
			return fmt.Errorf("invalid date parse: %s", err)
		}
		*d = Date(p)
		return nil
	case byte(bson.TypeDateTime):
		i, _, ok := bsoncore.ReadDateTime(v)
		if !ok {
			return fmt.Errorf("invalid bson datetime")
		}
		*d = Date(time.UnixMilli(i))
		return nil
	default:
		return fmt.Errorf("unsupported bson type for date: %v", t)
	}
}

type CNAE struct {
	Codigo    int    `json:"codigo" bson:"codigo"`
	Descricao string `json:"descricao" bson:"descricao"`
}

type TaxRegime struct {
	Ano                       int     `json:"ano" bson:"ano"`
	CNPJDaSCP                 *string `json:"cnpj_da_scp" bson:"cnpj_da_scp"`
	FormaDeTributação         string  `json:"forma_de_tributacao" bson:"forma_de_tributacao"`
	QuantidadeDeEscrituracoes int     `json:"quantidade_de_escrituracoes" bson:"quantidade_de_escrituracoes"`
}

type Partner struct {
	IdentificadorDeSocio                 *int    `json:"identificador_de_socio" bson:"identificador_de_socio"`
	NomeSocio                            string  `json:"nome_socio" bson:"nome_socio"`
	CNPJCPFDoSocio                       string  `json:"cnpj_cpf_do_socio" bson:"cnpj_cpf_do_socio"`
	CodigoQualificacaoSocio              *int    `json:"codigo_qualificacao_socio" bson:"codigo_qualificacao_socio"`
	QualificaoSocio                      *string `json:"qualificacao_socio" bson:"qualificacao_socio"`
	DataEntradaSociedade                 *Date   `json:"data_entrada_sociedade" bson:"data_entrada_sociedade"`
	CodigoPais                           *int    `json:"codigo_pais" bson:"codigo_pais"`
	Pais                                 *string `json:"pais" bson:"pais"`
	CPFRepresentanteLegal                string  `json:"cpf_representante_legal" bson:"cpf_representante_legal"`
	NomeRepresentanteLegal               string  `json:"nome_representante_legal" bson:"nome_representante_legal"`
	CodigoQualificacaoRepresentanteLegal *int    `json:"codigo_qualificacao_representante_legal" bson:"codigo_qualificacao_representante_legal"`
	QualificacaoRepresentanteLegal       *string `json:"qualificacao_representante_legal" bson:"qualificacao_representante_legal"`
	CodigoFaixaEtaria                    *int    `json:"codigo_faixa_etaria" bson:"codigo_faixa_etaria"`
	FaixaEtaria                          *string `json:"faixa_etaria" bson:"faixa_etaria"`
}

type Company struct {
	CNPJ                             string      `json:"cnpj" bson:"cnpj"`
	IdentificadorMatrizFilial        *int        `json:"identificador_matriz_filial" bson:"identificador_matriz_filial"`
	DescricaoMatrizFilial            *string     `json:"descricao_identificador_matriz_filial" bson:"descricao_identificador_matriz_filial"`
	NomeFantasia                     string      `json:"nome_fantasia" bson:"nome_fantasia"`
	SituacaoCadastral                *int        `json:"situacao_cadastral" bson:"situacao_cadastral"`
	DescricaoSituacaoCadastral       *string     `json:"descricao_situacao_cadastral" bson:"descricao_situacao_cadastral"`
	DataSituacaoCadastral            *Date       `json:"data_situacao_cadastral" bson:"data_situacao_cadastral"`
	MotivoSituacaoCadastral          *int        `json:"motivo_situacao_cadastral" bson:"motivo_situacao_cadastral"`
	DescricaoMotivoSituacaoCadastral *string     `json:"descricao_motivo_situacao_cadastral" bson:"descricao_motivo_situacao_cadastral"`
	NomeCidadeNoExterior             string      `json:"nome_cidade_no_exterior" bson:"nome_cidade_no_exterior"`
	CodigoPais                       *int        `json:"codigo_pais" bson:"codigo_pais"`
	Pais                             *string     `json:"pais" bson:"pais"`
	DataInicioAtividade              *Date       `json:"data_inicio_atividade" bson:"data_inicio_atividade"`
	CNAEFiscal                       *int        `json:"cnae_fiscal" bson:"cnae_fiscal"`
	CNAEFiscalDescricao              *string     `json:"cnae_fiscal_descricao" bson:"cnae_fiscal_descricao"`
	DescricaoTipoDeLogradouro        string      `json:"descricao_tipo_de_logradouro" bson:"descricao_tipo_de_logradouro"`
	Logradouro                       string      `json:"logradouro" bson:"logradouro"`
	Numero                           string      `json:"numero" bson:"numero"`
	Complemento                      string      `json:"complemento" bson:"complemento"`
	Bairro                           string      `json:"bairro" bson:"bairro"`
	CEP                              string      `json:"cep" bson:"cep"`
	UF                               string      `json:"uf" bson:"uf"`
	CodigoMunicipio                  *int        `json:"codigo_municipio" bson:"codigo_municipio"`
	CodigoMunicipioIBGE              *int        `json:"codigo_municipio_ibge" bson:"codigo_municipio_ibge"`
	Municipio                        *string     `json:"municipio" bson:"municipio"`
	Telefone1                        string      `json:"ddd_telefone_1" bson:"ddd_telefone_1"`
	Telefone2                        string      `json:"ddd_telefone_2" bson:"ddd_telefone_2"`
	Fax                              string      `json:"ddd_fax" bson:"ddd_fax"`
	Email                            *string     `json:"email" bson:"email"`
	SituacaoEspecial                 string      `json:"situacao_especial" bson:"situacao_especial"`
	DataSituacaoEspecial             *Date       `json:"data_situacao_especial" bson:"data_situacao_especial"`
	OpcaoPeloSimples                 *bool       `json:"opcao_pelo_simples" bson:"opcao_pelo_simples"`
	DataOpcaoPeloSimples             *Date       `json:"data_opcao_pelo_simples" bson:"data_opcao_pelo_simples"`
	DataExclusaoDoSimples            *Date       `json:"data_exclusao_do_simples" bson:"data_exclusao_do_simples"`
	OpcaoPeloMEI                     *bool       `json:"opcao_pelo_mei" bson:"opcao_pelo_mei"`
	DataOpcaoPeloMEI                 *Date       `json:"data_opcao_pelo_mei" bson:"data_opcao_pelo_mei"`
	DataExclusaoDoMEI                *Date       `json:"data_exclusao_do_mei" bson:"data_exclusao_do_mei"`
	RazaoSocial                      string      `json:"razao_social" bson:"razao_social"`
	CodigoNaturezaJuridica           *int        `json:"codigo_natureza_juridica" bson:"codigo_natureza_juridica"`
	NaturezaJuridica                 *string     `json:"natureza_juridica" bson:"natureza_juridica"`
	QualificacaoDoResponsavel        *int        `json:"qualificacao_do_responsavel" bson:"qualificacao_do_responsavel"`
	CapitalSocial                    *float32    `json:"capital_social" bson:"capital_social"`
	CodigoPorte                      *int        `json:"codigo_porte" bson:"codigo_porte"`
	Porte                            *string     `json:"porte" bson:"porte"`
	EnteFederativoResponsavel        string      `json:"ente_federativo_responsavel" bson:"ente_federativo_responsavel"`
	QuadroSocietario                 []Partner   `json:"qsa" bson:"qsa"`
	CNAESecundarios                  []CNAE      `json:"cnaes_secundarios" bson:"cnaes_secundarios"`
	RegimeTributario                 []TaxRegime `json:"regime_tributario" bson:"regime_tributario"`
}

// WithPrivacy masks sensitive personal details (MEI name, email, addresses/telephones for individual entities)
func (c *Company) WithPrivacy() {
	c.NomeFantasia = strings.TrimSpace(maskCPF(c.NomeFantasia))
	c.Email = nil
	if c.CodigoNaturezaJuridica != nil && c.NaturezaJuridica != nil && strings.Contains(strings.ToLower(*c.NaturezaJuridica), "individual") {
		c.DescricaoTipoDeLogradouro = ""
		c.Logradouro = ""
		c.Numero = ""
		c.Complemento = ""
		c.Telefone1 = ""
		c.Telefone2 = ""
		c.Fax = ""
	}
}

var jsonBufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 1024))
	},
}

// JSON serializes the company to JSON using an optimized buffer pool.
func (c *Company) JSON() ([]byte, error) {
	b := jsonBufferPool.Get().(*bytes.Buffer)
	defer func() {
		b.Reset()
		jsonBufferPool.Put(b)
	}()
	if err := json.MarshalWrite(b, c); err != nil {
		return nil, fmt.Errorf("error while marshaling company JSON: %w", err)
	}
	out := make([]byte, b.Len())
	copy(out, b.Bytes())
	return out, nil
}
