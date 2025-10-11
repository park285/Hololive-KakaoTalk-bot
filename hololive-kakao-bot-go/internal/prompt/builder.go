package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"sync"
	"text/template"
)

//go:embed templates/*.yaml
var templateFS embed.FS

type TemplateName string

const (
	TemplateParserPrompt                TemplateName = "parser_prompt.yaml"
	TemplateClarificationBasic          TemplateName = "clarification_basic.yaml"
	TemplateClarificationWithMembers    TemplateName = "clarification_with_members.yaml"
	TemplateClarificationWithoutMembers TemplateName = "clarification_without_members.yaml"
	TemplateChannelSelector             TemplateName = "channel_selector.yaml"
)

type PromptBuilder struct {
	mu        sync.RWMutex
	templates map[TemplateName]*template.Template
}

var (
	defaultBuilderOnce sync.Once
	defaultBuilder     *PromptBuilder
)

func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		templates: make(map[TemplateName]*template.Template),
	}
}

func DefaultPromptBuilder() *PromptBuilder {
	defaultBuilderOnce.Do(func() {
		defaultBuilder = NewPromptBuilder()
	})
	return defaultBuilder
}

func (pb *PromptBuilder) Render(name TemplateName, data any) (string, error) {
	tmpl, err := pb.getTemplate(name)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt %s: %w", name, err)
	}

	return buf.String(), nil
}

func (pb *PromptBuilder) getTemplate(name TemplateName) (*template.Template, error) {
	pb.mu.RLock()
	if tmpl, ok := pb.templates[name]; ok {
		pb.mu.RUnlock()
		return tmpl, nil
	}
	pb.mu.RUnlock()

	filename := filepath.ToSlash(filepath.Join("templates", string(name)))
	content, err := templateFS.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("load prompt template %s: %w", name, err)
	}

	tmpl, err := template.New(string(name)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parse prompt template %s: %w", name, err)
	}

	pb.mu.Lock()
	defer pb.mu.Unlock()
	pb.templates[name] = tmpl

	return tmpl, nil
}
