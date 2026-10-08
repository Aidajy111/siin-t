package model

import "time"

type Link struct {
	ID        string
	URL       string
	Clicks    int64
	CreatedAt time.Time
}
