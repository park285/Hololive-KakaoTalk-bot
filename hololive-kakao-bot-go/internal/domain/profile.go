package domain

// TalentProfile represents the raw data extracted from the official Hololive talent page.
type TalentProfile struct {
	Slug         string               `json:"slug"`
	EnglishName  string               `json:"english_name"`
	JapaneseName string               `json:"japanese_name"`
	Catchphrase  string               `json:"catchphrase"`
	Description  string               `json:"description"`
	DataEntries  []TalentProfileEntry `json:"data_entries"`
	SocialLinks  []TalentSocialLink   `json:"social_links"`
	OfficialURL  string               `json:"official_url"`
}

// TalentProfileEntry represents a single (label, value) pair in the profile data table.
type TalentProfileEntry struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// TalentSocialLink represents a labelled social link entry (e.g. YouTube, X).
type TalentSocialLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// TranslatedTalentProfile captures the AI-translated representation returned from the LLM.
type TranslatedTalentProfile struct {
	DisplayName string                     `json:"display_name"`
	Catchphrase string                     `json:"catchphrase"`
	Summary     string                     `json:"summary"`
	Highlights  []string                   `json:"highlights"`
	Data        []TranslatedProfileDataRow `json:"data"`
}

// TranslatedProfileDataRow represents a translated profile entry row.
type TranslatedProfileDataRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
