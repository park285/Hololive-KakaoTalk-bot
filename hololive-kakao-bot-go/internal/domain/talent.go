package domain

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

type OfficialTalent struct {
	Japanese string `json:"japanese"`
	English  string `json:"english"`
	Link     string `json:"link"`
	Status   string `json:"status"`
}

type Talents struct {
	Talents   []*OfficialTalent
	byEnglish map[string]*OfficialTalent
}

var officialTalentsJSON []byte

func LoadTalents() (*Talents, error) {
	var talents []*OfficialTalent
	if err := json.Unmarshal(officialTalentsJSON, &talents); err != nil {
		return nil, err
	}

	index := make(map[string]*OfficialTalent, len(talents))
	for _, talent := range talents {
		if talent == nil {
			continue
		}
		index[util.Normalize(talent.English)] = talent
	}

	return &Talents{
		Talents:   talents,
		byEnglish: index,
	}, nil
}

func (ot *Talents) FindByEnglish(name string) *OfficialTalent {
	if ot == nil {
		return nil
	}
	return ot.byEnglish[util.Normalize(name)]
}

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

	fallback := util.Normalize(ot.English)
	fallback = strings.ReplaceAll(fallback, " ", "-")
	fallback = strings.ReplaceAll(fallback, "'", "")
	fallback = strings.ReplaceAll(fallback, ".", "")
	return fallback
}
