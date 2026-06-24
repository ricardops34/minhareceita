SELECT count(*)
FROM {{ .CompanyTableFullName }},
LATERAL jsonb_array_elements({{ .CompanyTableFullName }}.{{ .JSONFieldName }}->'qsa') AS item
