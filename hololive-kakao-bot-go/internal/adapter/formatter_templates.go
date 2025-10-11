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
