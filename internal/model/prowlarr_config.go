package model

import "time"

const TableNameProwlarrConfig = "prowlarr_config"

type ProwlarrConfig struct {
	ID        int16     `gorm:"column:id;primaryKey"`
	Enabled   bool      `gorm:"column:enabled;not null"`
	BaseURL   string    `gorm:"column:base_url;not null"`
	APIKey    string    `gorm:"column:api_key;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (*ProwlarrConfig) TableName() string { return TableNameProwlarrConfig }
