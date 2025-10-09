package domain

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

type TalentProfileEntry struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type TalentSocialLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Translated struct {
	DisplayName string                     `json:"display_name"`
	Catchphrase string                     `json:"catchphrase"`
	Summary     string                     `json:"summary"`
	Highlights  []string                   `json:"highlights"`
	Data        []TranslatedProfileDataRow `json:"data"`
}

type TranslatedProfileDataRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
