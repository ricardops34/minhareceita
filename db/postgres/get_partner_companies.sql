SELECT partner_name, partner_cnpf, company_id, company_name, partner_type FROM {{ .GraphTableFullName }} WHERE partner_id = $1;
