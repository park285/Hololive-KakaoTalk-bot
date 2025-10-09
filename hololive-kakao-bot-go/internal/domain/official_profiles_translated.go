package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOfficialTranslatedProfiles loads pre-translated talent profiles (Korean) from disk.
func LoadOfficialTranslatedProfiles() (map[string]*TranslatedTalentProfile, error) {
	profilesDir := "internal/domain/data/official_profiles_ko"

	files, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read translated profiles directory: %w", err)
	}
	if len(files) == 0 {
		return map[string]*TranslatedTalentProfile{}, nil
	}

	profiles := make(map[string]*TranslatedTalentProfile, len(files))

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		slug := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		filePath := filepath.Join(profilesDir, file.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read translated profile %s: %w", file.Name(), err)
		}

		var profile TranslatedTalentProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("failed to parse translated profile %s: %w", file.Name(), err)
		}

		profiles[slug] = &profile
	}

	return profiles, nil
}
