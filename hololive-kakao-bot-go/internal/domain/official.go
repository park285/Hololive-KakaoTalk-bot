package domain

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"path"
	"strings"
)

// OfficialTalent represents a single talent entry from the official Hololive website dataset.
type OfficialTalent struct {
	Japanese string `json:"japanese"`
	English  string `json:"english"`
	Link     string `json:"link"`
	Status   string `json:"status"`
}

// OfficialTalents wraps the official talent dataset with helper indexes.
type OfficialTalents struct {
	Talents   []*OfficialTalent
	byEnglish map[string]*OfficialTalent
}

//go:embed data/official_talents.json
var officialTalentsJSON []byte

// LoadOfficialTalents loads the embedded official talent dataset.
func LoadOfficialTalents() (*OfficialTalents, error) {
	var talents []*OfficialTalent
	if err := json.Unmarshal(officialTalentsJSON, &talents); err != nil {
		return nil, err
	}

	index := make(map[string]*OfficialTalent, len(talents))
	for _, talent := range talents {
		if talent == nil {
			continue
		}
		index[strings.ToLower(strings.TrimSpace(talent.English))] = talent
	}

	return &OfficialTalents{
		Talents:   talents,
		byEnglish: index,
	}, nil
}

// FindByEnglish returns the official talent entry using a case-insensitive English name lookup.
func (ot *OfficialTalents) FindByEnglish(name string) *OfficialTalent {
	if ot == nil {
		return nil
	}
	return ot.byEnglish[strings.ToLower(strings.TrimSpace(name))]
}

// Slug returns the slug portion of the talent's official profile link.
func (ot *OfficialTalent) Slug() string {
	if ot == nil {
		return ""
	}

	if ot.Link == "" {
		return ""
	}

	u, err := url.Parse(ot.Link)
	if err == nil {
		segment := strings.Trim(path.Base(u.Path), "/")
		if segment != "" && segment != "." && segment != "/" {
			return segment
		}
	}

	// Fallback: derive from English name (lowercase, whitespace to hyphen)
	fallback := strings.ToLower(strings.TrimSpace(ot.English))
	fallback = strings.ReplaceAll(fallback, " ", "-")
	fallback = strings.ReplaceAll(fallback, "'", "")
	fallback = strings.ReplaceAll(fallback, ".", "")
	return fallback
}
