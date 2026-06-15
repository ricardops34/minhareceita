{{ if not .Logged }}ALTER TABLE {{ .CompanyTableFullName }} SET UNLOGGED;{{ end }}
