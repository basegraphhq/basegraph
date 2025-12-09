package common

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrEmptySlug  = errors.New("slug cannot be empty")
	nonSlugChars  = regexp.MustCompile(`[^a-z0-9]+`)
)

func Slugify(input, fallback string) (string, error) {
	slug := slugify(input)
	if slug == "" {
		slug = slugify(fallback)
	}
	if slug == "" {
		return "", ErrEmptySlug
	}
	return slug, nil
}

func slugify(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	slug := nonSlugChars.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}
