package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"tangled.org/cuducos.me/go-cnpj"
)

type logEntry struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
	CNPJ  string    `json:"cnpj"`
	Code  string    `json:"code"`
}

// Inconsistency holds a code and the CNPJs affected by it.
type Inconsistency struct {
	Code  string
	CNPJs []string
}

// Category groups inconsistencies by type.
type Category struct {
	Label           string
	Description     string
	Total           int
	Inconsistencies []Inconsistency
}

// ReportData holds everything needed to render the template.
type ReportData struct {
	Date       string
	Total      string
	Categories []Category
}

var ibgeMessages = map[string]bool{
	"unknown CodigoMunicipioIBGE": true,
}

var categoryLabels = map[string]string{
	"unknown MotivoSituacaoCadastral": "Motivo da Situação Cadastral",
	"unknown CodigoPais":              "Código do País",
	"unknown Pais":                    "País (em sócios)",
}

var categoryDescriptions = map[string]string{
	"unknown MotivoSituacaoCadastral": "Os dados do CNPJ contêm códigos de motivo da situação cadastral que não possuem descrição correspondente nos arquivos de tabelas auxiliares fornecidos pela própria Receita Federal.",
	"unknown CodigoPais":              "Os dados do CNPJ contêm códigos de país que não possuem nome correspondente nos arquivos de tabelas auxiliares fornecidos pela própria Receita Federal.",
	"unknown Pais":                    "Os dados de sócios contêm códigos de país que não possuem nome correspondente nos arquivos de tabelas auxiliares fornecidos pela própria Receita Federal.",
}

const reportTemplate = `# Pedido de Acesso à Informação

Pedido com base na Lei Federal nº 12.527/2011, Art. 10, §1º e §3º.

**Órgão destinatário:** Secretaria Especial da Receita Federal do Brasil

Em {{ .Date }}, ao processar os dados abertos do CNPJ, foram identificadas **{{ .Total }} inconsistências**: códigos presentes nos registros de empresas ou sócios sem correspondência nas tabelas auxiliares publicadas pela Receita Federal.

As inconsistências estão detalhadas a seguir.
{{ range $ci, $cat := .Categories }}{{ $catIdx := inc $ci }}
## {{ $catIdx }}. {{ $cat.Label }}

{{ $cat.Description }}

**Registros afetados: {{ $cat.Total | formatInt }}**
{{ range $ii, $inc := $cat.Inconsistencies }}{{ $incIdx := inc $ii }}
### {{ $catIdx }}.{{ $incIdx }}. Código ` + "`{{ $inc.Code }}`" + ` ({{ len $inc.CNPJs | formatInt }} {{ len $inc.CNPJs | pluralize "registro" "registros" }})
{{ range $inc.CNPJs }}
1. ` + "`{{ . | maskCNPJ }}`" + `
{{- end }}
{{ end }}
{{- end }}
{{ $reqBase := inc (len .Categories) }}
## {{ $reqBase }}. Requerimentos
{{ $reqBase }}.1. Os códigos listados acima são válidos e estão em uso pela Receita Federal?

{{ $reqBase }}.2. Por que as descrições correspondentes não constam nas tabelas auxiliares publicadas?

{{ $reqBase }}.3. Há previsão de correção dessas inconsistências?

{{ $reqBase }}.4. Qual o prazo para correção?

{{ $reqBase }}.5. Requeiro acesso a documentação atualizada que mapeie todos os códigos válidos para motivos de situação cadastral e para países utilizados nos registros do CNPJ.

Para facilitar a análise, solicito que a resposta utilize a mesma numeração apresentada neste documento.
`

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ".")
}

var tmplFuncs = template.FuncMap{
	"formatInt": formatInt,
	"maskCNPJ":  cnpj.Mask,
	"inc":       func(i int) int { return i + 1 },
	"pluralize": func(singular, plural string, n int) string {
		if n == 1 {
			return singular
		}
		return plural
	},
}

func parse(r io.Reader) (map[string]map[string][]string, time.Time, error) {
	grouped := map[string]map[string][]string{}
	var latest time.Time
	s := bufio.NewScanner(r)
	for s.Scan() {
		var e logEntry
		if err := json.Unmarshal(s.Bytes(), &e); err != nil {
			continue
		}
		if e.Level != "WARN" || ibgeMessages[e.Msg] {
			continue
		}
		if _, ok := categoryLabels[e.Msg]; !ok {
			continue
		}
		if e.Time.After(latest) {
			latest = e.Time
		}
		if grouped[e.Msg] == nil {
			grouped[e.Msg] = map[string][]string{}
		}
		grouped[e.Msg][e.Code] = append(grouped[e.Msg][e.Code], e.CNPJ)
	}
	return grouped, latest, s.Err()
}

func buildCategories(grouped map[string]map[string][]string) []Category {
	var cats []Category
	for msg, codes := range grouped {
		var items []Inconsistency
		var catTotal int
		for code, cnpjs := range codes {
			sort.Strings(cnpjs)
			items = append(items, Inconsistency{Code: code, CNPJs: cnpjs})
			catTotal += len(cnpjs)
		}
		sort.Slice(items, func(i, j int) bool {
			return len(items[i].CNPJs) > len(items[j].CNPJs)
		})
		cats = append(cats, Category{
			Label:           categoryLabels[msg],
			Description:     categoryDescriptions[msg],
			Total:           catTotal,
			Inconsistencies: items,
		})
	}
	sort.Slice(cats, func(i, j int) bool {
		return cats[i].Total > cats[j].Total
	})
	return cats
}

// Report reads the log file at path and writes the LAI report to w.
func Report(path string, w io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("could not open log file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("could not close", "path", path, "error", err)
		}
	}()

	grouped, logDate, err := parse(f)
	if err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}
	if len(grouped) == 0 {
		return fmt.Errorf("no relevant inconsistencies found in log file (IBGE-related entries are excluded)")
	}

	cats := buildCategories(grouped)
	data := ReportData{
		Date:       logDate.Format("02/01/2006"),
		Total:      formatInt(totalAffected(cats)),
		Categories: cats,
	}

	t := template.Must(template.New("report").Funcs(tmplFuncs).Parse(reportTemplate))
	return t.Execute(w, data)
}

func totalAffected(cats []Category) int {
	var n int
	for _, c := range cats {
		for _, inc := range c.Inconsistencies {
			n += len(inc.CNPJs)
		}
	}
	return n
}
