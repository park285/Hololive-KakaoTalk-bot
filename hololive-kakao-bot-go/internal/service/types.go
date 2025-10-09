package service

// ModelPreset represents the model usage preset
type ModelPreset string

const (
	PresetCreative ModelPreset = "creative" // 창의적 응답
	PresetPrecise  ModelPreset = "precise"  // 정확한 응답
	PresetBalanced ModelPreset = "balanced" // 균형잡힌 응답
)

// ModelConfig holds model configuration
type ModelConfig struct {
	Temperature      float32
	TopP             float32
	TopK             int
	MaxOutputTokens  int
	ResponseMimeType string // "application/json" or "text/plain"
}

// OpenAIConfig holds OpenAI-specific configuration
type OpenAIConfig struct {
	Temperature float32
	MaxTokens   int
	TopP        float32
}

// GenerateMetadata contains metadata about the generation
type GenerateMetadata struct {
	Provider     string
	Model        string
	UsedFallback bool
}

// GenerateOptions holds options for AI generation
type GenerateOptions struct {
	Model         string
	JSONMode      bool
	Overrides     *ModelConfig
	CachedContent string // Gemini CachedContent name for context caching
}

// GetPresetConfig returns the configuration for a preset
func GetPresetConfig(preset ModelPreset) ModelConfig {
	switch preset {
	case PresetCreative:
		return ModelConfig{
			Temperature:     0.7,
			TopP:            0.95,
			TopK:            40,
			MaxOutputTokens: 2048,
		}
	case PresetPrecise:
		return ModelConfig{
			Temperature:     0.1,
			TopP:            0.9,
			TopK:            20,
			MaxOutputTokens: 1024,
		}
	case PresetBalanced:
		return ModelConfig{
			Temperature:     0.1,
			TopP:            0.95,
			TopK:            40,
			MaxOutputTokens: 4096,
		}
	default:
		return GetPresetConfig(PresetBalanced)
	}
}

// GetOpenAIPresetConfig returns OpenAI configuration for a preset
func GetOpenAIPresetConfig(preset ModelPreset) OpenAIConfig {
	switch preset {
	case PresetCreative:
		return OpenAIConfig{
			Temperature: 0.7,
			MaxTokens:   2048,
			TopP:        0.95,
		}
	case PresetPrecise:
		return OpenAIConfig{
			Temperature: 0.1,
			MaxTokens:   1024,
			TopP:        0.9,
		}
	case PresetBalanced:
		return OpenAIConfig{
			Temperature: 0.1,
			MaxTokens:   4096,
			TopP:        0.95,
		}
	default:
		return GetOpenAIPresetConfig(PresetBalanced)
	}
}
