package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type profileRefineFromOriginalTemplateData struct {
	Prompt string `json:"prompt"`
}

//go:embed templates/profile_refine_from_original_prompt.json
var profileRefineFromOriginalPromptJSON []byte

var profileRefineFromOriginalTemplate *template.Template

func init() {
	var data profileRefineFromOriginalTemplateData
	if err := json.Unmarshal(profileRefineFromOriginalPromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("profile_refine_from_original").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	profileRefineFromOriginalTemplate = tmpl
}

// ProfileRefineFromOriginalPromptVars provides original Japanese and current Korean translation.
type ProfileRefineFromOriginalPromptVars struct {
	OriginalCatchphrase string
	OriginalDescription string
	CurrentTranslation  string
}

// BuildProfileRefineFromOriginalPrompt renders the refinement prompt with original Japanese reference.
func BuildProfileRefineFromOriginalPrompt(vars ProfileRefineFromOriginalPromptVars) (string, error) {
	var buf bytes.Buffer
	if err := profileRefineFromOriginalTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}
