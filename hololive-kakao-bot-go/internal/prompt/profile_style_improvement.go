package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type profileStyleImprovementTemplateData struct {
	Prompt string `json:"prompt"`
}

//go:embed templates/profile_style_improvement_prompt.json
var profileStyleImprovementPromptJSON []byte

var profileStyleImprovementTemplate *template.Template

func init() {
	var data profileStyleImprovementTemplateData
	if err := json.Unmarshal(profileStyleImprovementPromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("profile_style_improvement").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	profileStyleImprovementTemplate = tmpl
}

// ProfileStyleImprovementPromptVars provides the current translation to be improved.
type ProfileStyleImprovementPromptVars struct {
	CurrentTranslation string
}

// BuildProfileStyleImprovementPrompt renders the style improvement prompt.
func BuildProfileStyleImprovementPrompt(vars ProfileStyleImprovementPromptVars) (string, error) {
	var buf bytes.Buffer
	if err := profileStyleImprovementTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
