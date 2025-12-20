package mapper

import (
	"context"
	"fmt"
)

type LinearEventMapper struct{}

func NewLinearEventMapper() *LinearEventMapper {
	return &LinearEventMapper{}
}

func (m *LinearEventMapper) Map(ctx context.Context, body map[string]any, headers map[string]string) (CanonicalEventType, error) {
	return "", fmt.Errorf("linear webhook mapper not implemented yet")
}
