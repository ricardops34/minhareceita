{{ if not .Logged }}ALTER TABLE {{ .CompanyTableFullName }} SET LOGGED;{{ end }}
