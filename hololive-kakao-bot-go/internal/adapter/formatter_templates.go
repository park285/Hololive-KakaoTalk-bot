package adapter

import (
	"embed"
	"strings"
	sync "sync"
	"text/template"
)

//go:embed templates/*.tmpl
var formatterTemplateFS embed.FS

var (
	formatterTemplates *template.Template
	formatterOnce      sync.Once
	formatterErr       error
)

func executeFormatterTemplate(name string, data any) (string, error) {
	formatterOnce.Do(func() {
		funcMap := template.FuncMap{
			"add": func(a, b int) int { return a + b },
			"indent": func(spaces int, text string) string {
				trimmed := strings.TrimSpace(text)
				if trimmed == "" {
					return ""
				}

				indent := strings.Repeat(" ", spaces)
				lines := strings.Split(text, "\n")
				for i, line := range lines {
					if strings.TrimSpace(line) == "" {
						lines[i] = indent
						continue
					}
					lines[i] = indent + line
				}
				return strings.Join(lines, "\n")
			},
		}
		tmpl := template.New("formatter").Funcs(funcMap)
		formatterTemplates, formatterErr = tmpl.ParseFS(formatterTemplateFS, "templates/*.tmpl")
	})

	if formatterErr != nil {
		return "", formatterErr
	}

	var builder strings.Builder
	if err := formatterTemplates.ExecuteTemplate(&builder, name, data); err != nil {
		return "", err
	}

	return strings.TrimRight(builder.String(), "\n"), nil
}
