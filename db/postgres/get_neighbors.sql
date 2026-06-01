SELECT company_id, company_name, partner_id, partner_name, partner_cnpf, partner_type
FROM {{ .GraphTableFullName }}
WHERE company_id = $1
UNION ALL
SELECT company_id, company_name, partner_id, partner_name, partner_cnpf, partner_type
FROM {{ .GraphTableFullName }}
WHERE partner_id = $1;
