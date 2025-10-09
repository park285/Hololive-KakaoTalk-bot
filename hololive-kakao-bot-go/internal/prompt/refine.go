package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type refineTemplateData struct {
	Prompt string `json:"prompt"`
}

var refinePromptJSON []byte

var refineTemplate *template.Template

func init() {
	if len(refinePromptJSON) == 0 {
		tmpl, err := template.New("refine").Parse("{{.CurrentTranslation}}")
		if err != nil {
			panic(err)
		}
		refineTemplate = tmpl
		return
	}

	var data refineTemplateData
	if err := json.Unmarshal(refinePromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("refine").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	refineTemplate = tmpl
}

type RefineVars struct {
	OriginalCatchphrase string
	OriginalDescription string
	CurrentTranslation  string
}

func BuildRefine(vars RefineVars) (string, error) {
	var buf bytes.Buffer
	if err := refineTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
