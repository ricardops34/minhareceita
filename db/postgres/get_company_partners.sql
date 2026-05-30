SELECT company_name, partner_id, partner_name, partner_cnpf, partner_type FROM {{ .GraphTableFullName }} WHERE company_id = $1;
