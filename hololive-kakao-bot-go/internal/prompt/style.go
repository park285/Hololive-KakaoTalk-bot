package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type styleTemplateData struct {
	Prompt string `json:"prompt"`
}

var stylePromptJSON []byte

var styleTemplate *template.Template

func init() {
	if len(stylePromptJSON) == 0 {
		tmpl, err := template.New("style").Parse("{{.CurrentTranslation}}")
		if err != nil {
			panic(err)
		}
		styleTemplate = tmpl
		return
	}

	var data styleTemplateData
	if err := json.Unmarshal(stylePromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("style").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	styleTemplate = tmpl
}

type StyleVars struct {
	CurrentTranslation string
}

func BuildStyle(vars StyleVars) (string, error) {
	var buf bytes.Buffer
	if err := styleTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
