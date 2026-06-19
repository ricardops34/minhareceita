package transform

import (
	"fmt"
	"log/slog"
	"strings"

	"codeberg.org/cuducos/minha-receita/company"
)

func newCompany(log *slog.Logger, srcs map[string]*source, kv *kv, row []string) (*company.Company, error) {
	var c company.Company
	var err error
	c.CNPJ = strings.Join(row[:3], "")
	log = log.With("cnpj", c.CNPJ)
	c.IdentificadorMatrizFilial, err = toInt(row[3])
	if err != nil {
		return nil, fmt.Errorf("could not parse IdentificadorMatrizFilial for %s: %w", c.CNPJ, err)
	}
	if err := descricaoMatrizFilial(&c, log); err != nil {
		return nil, fmt.Errorf("could not parse IdentificadorMatrizFilial for %s: %w", c.CNPJ, err)
	}
	c.NomeFantasia = row[4]
	c.SituacaoCadastral, err = toInt(row[5])
	if err != nil {
		return nil, fmt.Errorf("could not parse SituacaoCadastral for %s: %w", c.CNPJ, err)
	}
	if err := descricaoSituacaoCadastral(&c, log); err != nil {
		return nil, fmt.Errorf("could not get DescricaoSituacaoCadastral for %s: %w", c.CNPJ, err)
	}
	c.DataSituacaoCadastral, err = toDate(row[6])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataSituacaoCadastral for %s: %w", c.CNPJ, err)
	}
	c.MotivoSituacaoCadastral, err = toInt(row[7])
	if err != nil {
		return nil, fmt.Errorf("could not parse MotivoSituacaoCadastral for %s: %w", c.CNPJ, err)
	}
	c.DescricaoMotivoSituacaoCadastral, err = stringFromKV(srcs, kv, "mot", row[7], 0)
	if err != nil {
		log.Warn("unknown MotivoSituacaoCadastral", "code", row[7])
	}
	c.NomeCidadeNoExterior = row[8]
	c.CodigoPais, err = toInt(row[9])
	if err != nil {
		return nil, fmt.Errorf("could not parse CodigoPais for %s: %w", c.CNPJ, err)
	}
	c.Pais, err = stringFromKV(srcs, kv, "pai", row[9], 0)
	if err != nil {
		log.Warn("unknown CodigoPais", "code", row[9])
	}
	c.DataInicioAtividade, err = toDate(row[10])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataInicioAtividade for %s: %w", c.CNPJ, err)
	}
	c.CNAEFiscal, err = toInt(row[11])
	if err != nil {
		return nil, fmt.Errorf("could not parse CNAEFiscal for %s: %w", c.CNPJ, err)
	}
	c.CNAEFiscalDescricao, err = stringFromKV(srcs, kv, "cna", row[11], 0)
	if err != nil {
		return nil, fmt.Errorf("could not parse CNAEFiscalDescricao for %s: %w", c.CNPJ, err)
	}
	c.DescricaoTipoDeLogradouro = row[13]
	c.Logradouro = row[14]
	c.Numero = row[15]
	c.Complemento = row[16]
	c.Bairro = row[17]
	c.CEP = row[18]
	c.UF = row[19]
	c.CodigoMunicipio, err = toInt(row[20])
	if err != nil {
		return nil, fmt.Errorf("could not parse CodigoMunicipio for %s: %w", c.CNPJ, err)
	}
	if c.CodigoMunicipio != nil && *c.CodigoMunicipio != 9707 { // overseas city code
		ibge, err := stringFromKV(srcs, kv, "tab", row[20], 3)
		if err != nil {
			log.Warn("unknown CodigoMunicipioIBGE", "code", row[20])
		} else {
			c.CodigoMunicipioIBGE, err = toInt(*ibge)
			if err != nil {
				return nil, fmt.Errorf("could not parse CodigoMunicipioIBGE number for %s: %w", c.CNPJ, err)
			}
		}
	}
	c.Municipio, err = stringFromKV(srcs, kv, "mun", row[20], 0)
	if err != nil {
		log.Warn("unknown Municipio", "code", row[20])
	}
	c.Telefone1 = row[21] + row[22]
	c.Telefone2 = row[23] + row[24]
	c.Fax = row[25] + row[26]
	c.Email = &row[27]
	c.SituacaoEspecial = row[28]
	c.DataSituacaoEspecial, err = toDate(row[29])
	if err != nil {
		return nil, fmt.Errorf("could not parse DataSituacaoEspecial for %s: %w", c.CNPJ, err)
	}
	if err := base(&c, log, srcs, kv); err != nil {
		return nil, err
	}
	if err := simples(&c, srcs, kv); err != nil {
		return nil, err
	}
	if err := cnaes(&c, srcs, kv, row[12]); err != nil {
		return nil, err
	}
	if err := partners(&c, log, srcs, kv); err != nil {
		return nil, err
	}
	if err := taxes(&c, srcs, kv); err != nil {
		return nil, err
	}
	return &c, nil
}
