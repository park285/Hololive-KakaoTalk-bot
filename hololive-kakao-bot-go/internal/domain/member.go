package domain

import (
	_ "embed"
	"encoding/json"
)

type Aliases struct {
	Ko []string `json:"ko,omitempty"`
	Ja []string `json:"ja,omitempty"`
}

type Member struct {
	ChannelID   string   `json:"channelId"`
	Name        string   `json:"name"`
	Aliases     *Aliases `json:"aliases,omitempty"`
	NameJa      string   `json:"nameJa,omitempty"`
	NameKo      string   `json:"nameKo,omitempty"`
	IsGraduated bool     `json:"isGraduated,omitempty"`
}

type MembersData struct {
	Version     string    `json:"version"`
	LastUpdated string    `json:"lastUpdated"`
	Sources     []string  `json:"sources"`
	Members     []*Member `json:"members"`

	byChannelID map[string]*Member
	byName      map[string]*Member
}

//go:embed data/members.json
var membersJSON []byte

func (m *Member) GetAllAliases() []string {
	if m.Aliases == nil {
		return []string{}
	}

	all := make([]string, 0, len(m.Aliases.Ko)+len(m.Aliases.Ja))
	all = append(all, m.Aliases.Ko...)
	all = append(all, m.Aliases.Ja...)
	return all
}

func (m *Member) HasAlias(name string) bool {
	aliases := m.GetAllAliases()
	for _, alias := range aliases {
		if alias == name {
			return true
		}
	}
	return false
}

func LoadMembersData() (*MembersData, error) {
	var data MembersData
	if err := json.Unmarshal(membersJSON, &data); err != nil {
		return nil, err
	}

	data.byChannelID = make(map[string]*Member, len(data.Members))
	data.byName = make(map[string]*Member, len(data.Members))

	for _, member := range data.Members {
		data.byChannelID[member.ChannelID] = member
		data.byName[member.Name] = member
	}

	return &data, nil
}

func (md *MembersData) FindMemberByChannelID(channelID string) *Member {
	return md.byChannelID[channelID]
}

func (md *MembersData) FindMemberByName(name string) *Member {
	return md.byName[name]
}

func (md *MembersData) FindMemberByAlias(alias string) *Member {
	for _, member := range md.Members {
		if member.HasAlias(alias) {
			return member
		}
	}
	return nil
}

func (md *MembersData) GetChannelIDs() []string {
	ids := make([]string, len(md.Members))
	for i, member := range md.Members {
		ids[i] = member.ChannelID
	}
	return ids
}
