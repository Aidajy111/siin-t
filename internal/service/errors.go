package service

import "errors"

var (
	ErrInvalidURL            = errors.New("url must be an absolute HTTP(S) URL with a valid host and port")
	ErrLinkNotFound          = errors.New("link not found")
	ErrIDGenerationExhausted = errors.New("could not generate a unique link ID")
)
