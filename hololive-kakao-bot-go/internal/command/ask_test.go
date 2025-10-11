package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/ai"
	"go.uber.org/zap"
)

type fakeParser struct {
	result                *domain.ParseResults
	metadata              *ai.GenerateMetadata
	err                   error
	calls                 []string
	clarificationMessage  string
	clarificationMetadata *ai.GenerateMetadata
	clarificationErr      error
	smartClarification    *domain.Clarification
	smartMetadata         *ai.GenerateMetadata
	smartErr              error
}

type fakeDispatcher struct {
	events []CommandEvent
	err    error
}

func (f *fakeDispatcher) Publish(_ context.Context, _ *domain.CommandContext, events ...CommandEvent) (int, error) {
	for _, event := range events {
		if event.Params != nil {
			event.Params["__mutated__"] = true
		}
	}
	f.events = append(f.events, events...)
	return len(events), f.err
}

func (f *fakeParser) ParseNaturalLanguage(_ context.Context, query string, _ domain.MemberDataProvider) (*domain.ParseResults, *ai.GenerateMetadata, error) {
	f.calls = append(f.calls, query)
	return f.result, f.metadata, f.err
}

func (f *fakeParser) GenerateClarificationMessage(_ context.Context, _ string) (string, *ai.GenerateMetadata, error) {
	if f.clarificationErr != nil {
		return "", nil, f.clarificationErr
	}
	if f.clarificationMessage != "" {
		return f.clarificationMessage, f.clarificationMetadata, nil
	}
	return "", nil, nil
}

func (f *fakeParser) ClassifyMemberInfoIntent(_ context.Context, _ string) (*domain.MemberIntent, *ai.GenerateMetadata, error) {
	return &domain.MemberIntent{
		Intent:     domain.MemberIntentOther,
		Confidence: 0.9,
		Reasoning:  "test mock",
	}, nil, nil
}

func (f *fakeParser) GenerateSmartClarification(_ context.Context, _ string, _ domain.MemberDataProvider) (*domain.Clarification, *ai.GenerateMetadata, error) {
	if f.smartErr != nil {
		return nil, nil, f.smartErr
	}

	if f.smartClarification != nil {
		return f.smartClarification, f.smartMetadata, nil
	}

	return &domain.Clarification{
		IsHololiveRelated: false,
		Message:           "",
		Candidate:         "",
	}, nil, nil
}

func TestAskCommandDelegatesParsedCommands(t *testing.T) {
	parser := &fakeParser{
		result: &domain.ParseResults{
			Multiple: []*domain.ParseResult{
				{
					Command:    domain.CommandSchedule,
					Confidence: 0.9,
					Params: map[string]any{
						"member": "Usada Pekora",
					},
				},
				{
					Command:    domain.CommandAsk,
					Confidence: 0.9,
					Params:     map[string]any{},
				},
			},
		},
		metadata: &ai.GenerateMetadata{
			Provider:     "Gemini",
			Model:        "test-model",
			UsedFallback: false,
		},
	}

	dispatcher := &fakeDispatcher{}
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, message string) error {
			t.Fatalf("unexpected SendMessage call: %s", message)
			return nil
		},
		SendError: func(room, message string) error {
			t.Fatalf("unexpected SendError call: %s", message)
			return nil
		},
		Dispatcher: dispatcher,
		Logger:     zap.NewNop(),
	}

	cmd := NewAskCommand(deps)
	err := cmd.Execute(context.Background(), domain.NewCommandContext("room", "room", "user", "!ask", false), map[string]any{
		"question": "페코라 일정 알려줘",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(parser.calls) != 1 {
		t.Fatalf("expected parser to be called once, got %d", len(parser.calls))
	}
	if len(dispatcher.events) != 1 || dispatcher.events[0].Type != domain.CommandSchedule {
		t.Fatalf("expected CommandSchedule to be dispatched once, got %v", dispatcher.events)
	}

	if parser.result.Multiple[0].Params["member"] != "Usada Pekora" {
		t.Fatalf("expected original params to remain unchanged, got %v", parser.result.Multiple[0].Params["member"])
	}

	if _, ok := dispatcher.events[0].Params["__mutated__"]; !ok {
		t.Fatalf("expected dispatcher to record mutation sentinel")
	}
}

func TestAskCommandHandlesUnknownResults(t *testing.T) {
	parser := &fakeParser{
		result: &domain.ParseResults{
			Single: &domain.ParseResult{
				Command:    domain.CommandUnknown,
				Confidence: 0.9,
				Params:     map[string]any{},
			},
		},
	}

	var message string
	dispatcher := &fakeDispatcher{}
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			message = msg
			return nil
		},
		SendError: func(room, msg string) error {
			t.Fatalf("unexpected SendError call: %s", msg)
			return nil
		},
		Dispatcher: dispatcher,
		Logger:     zap.NewNop(),
	}

	cmd := NewAskCommand(deps)
	err := cmd.Execute(context.Background(), domain.NewCommandContext("room", "room", "user", "!ask", false), map[string]any{
		"question": "??",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(message, "요청을 이해하지 못했습니다") {
		t.Fatalf("unexpected fallback message: %s", message)
	}
}

func TestAskCommandHandlesParserError(t *testing.T) {
	parser := &fakeParser{
		err: fmt.Errorf("temporary AI failure"),
	}

	var errorMsg string
	dispatcher := &fakeDispatcher{}
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			t.Fatalf("unexpected SendMessage call: %s", msg)
			return nil
		},
		SendError: func(room, msg string) error {
			errorMsg = msg
			return nil
		},
		Dispatcher: dispatcher,
		Logger:     zap.NewNop(),
	}

	cmd := NewAskCommand(deps)
	err := cmd.Execute(context.Background(), domain.NewCommandContext("room", "room", "user", "!ask", false), map[string]any{
		"question": "페코라 일정 알려줘",
	})
	if err != nil {
		t.Fatalf("expected AskCommand to swallow SendError result, got %v", err)
	}

	if !strings.Contains(errorMsg, "temporary AI failure") {
		t.Fatalf("expected propagated error message, got %s", errorMsg)
	}
}

func TestAskCommandValidatesEmptyQuestion(t *testing.T) {
	parser := &fakeParser{}
	var errorMsg string
	dispatcher := &fakeDispatcher{}
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			t.Fatalf("unexpected SendMessage call")
			return nil
		},
		SendError: func(room, msg string) error {
			errorMsg = msg
			return nil
		},
		Dispatcher: dispatcher,
		Logger:     zap.NewNop(),
	}

	cmd := NewAskCommand(deps)
	err := cmd.Execute(context.Background(), domain.NewCommandContext("room", "room", "user", "!ask", false), map[string]any{
		"question": "   ",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(parser.calls) != 0 {
		t.Fatalf("expected parser not to be invoked for empty question, got %d calls", len(parser.calls))
	}
	if !strings.Contains(errorMsg, "질문을 이해하지 못했습니다") {
		t.Fatalf("unexpected validation message: %s", errorMsg)
	}

	if len(dispatcher.events) != 0 {
		t.Fatalf("dispatcher should not be invoked for empty question, got %v", dispatcher.events)
	}
}

func TestAskCommandSkipsLowConfidenceResults(t *testing.T) {
	parser := &fakeParser{
		result: &domain.ParseResults{
			Single: &domain.ParseResult{
				Command:    domain.CommandSchedule,
				Confidence: 0.2,
				Params: map[string]any{
					"member": "Usada Pekora",
				},
			},
		},
	}

	var message string
	dispatcher := &fakeDispatcher{}
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			message = msg
			return nil
		},
		SendError: func(room, msg string) error {
			t.Fatalf("unexpected SendError call: %s", msg)
			return nil
		},
		Dispatcher: dispatcher,
		Logger:     zap.NewNop(),
	}

	cmd := NewAskCommand(deps)
	err := cmd.Execute(context.Background(), domain.NewCommandContext("room", "room", "user", "!ask", false), map[string]any{
		"question": "페코라 일정 알려줘",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(message, "요청을 이해하지 못했습니다") {
		t.Fatalf("unexpected fallback message: %s", message)
	}

	if len(dispatcher.events) != 0 {
		t.Fatalf("dispatcher should not be invoked for low-confidence results, got %v", dispatcher.events)
	}
}

func TestHandleMemberFallbackFailurePrefersLLMMessage(t *testing.T) {
	parser := &fakeParser{
		smartClarification: &domain.Clarification{
			IsHololiveRelated: true,
			Message:           `누구를 말씀하신 건지 잘 모르겠어요. "하짱"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.`,
		},
		smartMetadata: &ai.GenerateMetadata{
			Provider:     "Gemini",
			Model:        "test",
			UsedFallback: false,
		},
	}

	var sent string
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			sent = msg
			return nil
		},
		Logger: zap.NewNop(),
	}

	wf := &askWorkflow{
		ctx:      context.Background(),
		deps:     deps,
		cmdCtx:   domain.NewCommandContext("room", "room", "user", "!ask", false),
		provider: deps.MembersData.WithContext(context.Background()),
		logger:   deps.Logger,
	}

	ok := wf.handleFallbackFail("하짱 알려줘")
	if !ok {
		t.Fatalf("expected clarification handler to succeed")
	}
	if sent != `누구를 말씀하신 건지 잘 모르겠어요. "하짱"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.` {
		t.Fatalf("expected LLM clarification message, got %q", sent)
	}
}

func TestHandleMemberFallbackFailureFallsBackToTemplate(t *testing.T) {
	parser := &fakeParser{
		smartErr: errors.New("llm failure"),
	}

	var sent string
	deps := &Dependencies{
		Gemini:      parser,
		MembersData: &domain.MembersData{},
		SendMessage: func(room, msg string) error {
			sent = msg
			return nil
		},
		Logger: zap.NewNop(),
	}

	wf := &askWorkflow{
		ctx:      context.Background(),
		deps:     deps,
		cmdCtx:   domain.NewCommandContext("room", "room", "user", "!ask", false),
		provider: deps.MembersData.WithContext(context.Background()),
		logger:   deps.Logger,
	}

	ok := wf.handleFallbackFail(`하 "짱" 알려줘`)
	if !ok {
		t.Fatalf("expected clarification handler to succeed with template fallback")
	}
	expected := `누구를 말씀하신 건지 잘 모르겠어요. "하 '짱' 알려줘"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.`
	if sent != expected {
		t.Fatalf("expected template fallback message, got %q", sent)
	}
}
