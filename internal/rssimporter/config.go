package rssimporter

import "time"

type FeedConfig struct {
	URL    string
	Source string
}

type Config struct {
	Feeds        []FeedConfig
	PollInterval time.Duration
}

func NewDefaultConfig() Config {
	return Config{
		Feeds:        []FeedConfig{},
		PollInterval: 30 * time.Minute,
	}
}
