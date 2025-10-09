package domain

import (
	"embed"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed data/official_profiles_raw/*.json
var officialProfilesRawFS embed.FS

func LoadProfiles() (map[string]*TalentProfile, error) {
	files, err := officialProfilesRawFS.ReadDir("data/official_profiles_raw")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded profiles: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no embedded profiles found")
	}

	profiles := make(map[string]*TalentProfile, len(files))

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		slug := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
		data, err := officialProfilesRawFS.ReadFile(filepath.Join("data/official_profiles_raw", file.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read profile %s: %w", file.Name(), err)
		}

		var profile TalentProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("failed to parse profile %s: %w", file.Name(), err)
		}

		if profile.Slug == "" {
			profile.Slug = slug
		}
		profiles[slug] = &profile
	}

	return profiles, nil
}
