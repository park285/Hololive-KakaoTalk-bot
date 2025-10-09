package domain

type Channel struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	EnglishName     *string `json:"english_name,omitempty"`
	Photo           *string `json:"photo,omitempty"`
	Twitter         *string `json:"twitter,omitempty"`
	VideoCount      *int    `json:"video_count,omitempty"`
	SubscriberCount *int    `json:"subscriber_count,omitempty"`
	Org             *string `json:"org,omitempty"`
	Suborg          *string `json:"suborg,omitempty"`
	Group           *string `json:"group,omitempty"`
}

func (c *Channel) GetDisplayName() string {
	if c == nil {
		return ""
	}
	if c.EnglishName != nil && *c.EnglishName != "" {
		return *c.EnglishName
	}
	return c.Name
}

func (c *Channel) IsHololive() bool {
	if c == nil || c.Org == nil {
		return false
	}
	return *c.Org == "Hololive"
}

func (c *Channel) HasPhoto() bool {
	if c == nil {
		return false
	}
	return c.Photo != nil && *c.Photo != ""
}

func (c *Channel) GetPhotoURL() string {
	if c == nil {
		return ""
	}
	if c.HasPhoto() {
		return *c.Photo
	}
	return ""
}
