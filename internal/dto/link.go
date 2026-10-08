package dto

import "time"

type CreateLinkRequest struct {
	URL string `json:"url"`
}

type CreateLinkResponse struct {
	ID       string `json:"id"`
	ShortURL string `json:"short_url"`
}

type LinkResponse struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
}
