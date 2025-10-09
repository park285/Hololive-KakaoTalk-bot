package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type translateTemplateData struct {
	Prompt string `json:"prompt"`
}

var translatePromptJSON []byte

var translateTemplate *template.Template

func init() {
	if len(translatePromptJSON) == 0 {
		tmpl, err := template.New("translate").Parse("{{.Description}}")
		if err != nil {
			panic(err)
		}
		translateTemplate = tmpl
		return
	}

	var data translateTemplateData
	if err := json.Unmarshal(translatePromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("translate").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	translateTemplate = tmpl
}

type TranslateEntry struct {
	Label string
	Value string
}

type TranslateVars struct {
	EnglishName    string
	JapaneseName   string
	Catchphrase    string
	Description    string
	DataEntries    []TranslateEntry
	MaxDataEntries int
}

func BuildTranslate(vars TranslateVars) (string, error) {
	if vars.MaxDataEntries <= 0 {
		vars.MaxDataEntries = len(vars.DataEntries)
	}

	var buf bytes.Buffer
	if err := translateTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
