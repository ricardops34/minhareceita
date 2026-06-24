COPY (
  SELECT
    {{ .IDFieldName }} AS company_id,
    {{ .JSONFieldName }}->>'razao_social' AS company_name,
    CASE
      WHEN item->>'identificador_de_socio' = '1' THEN item->>'cnpj_cpf_do_socio'
      ELSE md5(concat(item->>'cnpj_cpf_do_socio', item->>'nome_socio'))
    END AS partner_id,
    COALESCE(item->>'nome_socio', '') AS partner_name,
    COALESCE(item->>'cnpj_cpf_do_socio', '') AS partner_cnpf,
    COALESCE((item->>'identificador_de_socio')::int, 0) AS partner_type
  FROM {{ .CompanyTableFullName }},
  LATERAL jsonb_array_elements({{ .CompanyTableFullName }}.{{ .JSONFieldName }}->'qsa') AS item
) TO STDOUT WITH CSV
