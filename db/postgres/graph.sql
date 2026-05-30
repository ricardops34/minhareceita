CREATE TABLE IF NOT EXISTS {{ .GraphTableFullName }} AS
SELECT
    {{ .IDFieldName }} AS company_id,
    CASE
        WHEN item->>'identificador_de_socio' = '1' THEN item->>'cnpj_cpf_do_socio'
        ELSE md5(concat(item->>'cnpj_cpf_do_socio', item->>'nome_socio'))
    END AS partner_id,
    {{ .JSONFieldName }}->>'razao_social' AS company_name,
    item->>'nome_socio' AS partner_name,
    item->>'cnpj_cpf_do_socio' AS partner_cnpf,
    (item->>'identificador_de_socio')::int AS partner_type
FROM {{ .CompanyTableFullName }},
LATERAL jsonb_array_elements({{ .CompanyTableFullName }}.{{ .JSONFieldName }}->'qsa') AS item;

CREATE INDEX IF NOT EXISTS idx_graph_partner_id ON {{ .GraphTableFullName }} (partner_id);
CREATE INDEX IF NOT EXISTS idx_graph_company_id ON {{ .GraphTableFullName }} (company_id);
