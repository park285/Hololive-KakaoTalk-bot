package prompt

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"text/template"
)

type profileTranslationTemplateData struct {
	Prompt string `json:"prompt"`
}

//go:embed templates/profile_translation_prompt.json
var profileTranslationPromptJSON []byte

var profileTranslationTemplate *template.Template

func init() {
	var data profileTranslationTemplateData
	if err := json.Unmarshal(profileTranslationPromptJSON, &data); err != nil {
		panic(err)
	}

	tmpl, err := template.New("profile_translation").Parse(data.Prompt)
	if err != nil {
		panic(err)
	}

	profileTranslationTemplate = tmpl
}

// ProfileTranslationPromptEntry represents a single key/value pair that should appear in the prompt.
type ProfileTranslationPromptEntry struct {
	Label string
	Value string
}

// ProfileTranslationPromptVars provides dynamic values for building the translation prompt.
type ProfileTranslationPromptVars struct {
	EnglishName    string
	JapaneseName   string
	Catchphrase    string
	Description    string
	DataEntries    []ProfileTranslationPromptEntry
	MaxDataEntries int
}

// BuildProfileTranslationPrompt renders the translation prompt using the embedded template.
func BuildProfileTranslationPrompt(vars ProfileTranslationPromptVars) (string, error) {
	if vars.MaxDataEntries <= 0 {
		vars.MaxDataEntries = len(vars.DataEntries)
	}

	var buf bytes.Buffer
	if err := profileTranslationTemplate.Execute(&buf, vars); err != nil {
		return "", err
	}

	return buf.String(), nil
}

