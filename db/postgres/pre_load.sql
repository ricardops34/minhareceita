{{ if not .Logged }}ALTER TABLE {{ .CompanyTableFullName }} SET UNLOGGED;{{ end }}
DROP INDEX IF EXISTS {{ .CompanyTableFullName }}_id;
