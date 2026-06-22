package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const TableNameRssFeed = "rss_feeds"

type RssFeed struct {
	ID        string    `gorm:"column:id;primaryKey"`
	URL       string    `gorm:"column:url;not null"`
	Source    string    `gorm:"column:source;not null;uniqueIndex"`
	CreatedAt time.Time `gorm:"column:created_at;not null;<-:create"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (*RssFeed) TableName() string {
	return TableNameRssFeed
}

func (f *RssFeed) BeforeCreate(_ *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return nil
}
