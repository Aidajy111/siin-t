package repository

import "errors"

var (
	ErrLinkNotFound = errors.New("link not found")
	ErrIDExists     = errors.New("link ID already exists")
)
