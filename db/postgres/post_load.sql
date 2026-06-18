CREATE UNIQUE INDEX IF NOT EXISTS {{ .CompanyTableName }}_id ON {{ .CompanyTableFullName }} ({{ .IDFieldName }});
{{ if not .Logged }}ALTER TABLE {{ .CompanyTableFullName }} SET LOGGED;{{ end }}
