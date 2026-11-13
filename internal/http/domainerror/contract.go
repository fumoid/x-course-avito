package domainerror

import "context"

type logger interface {
	ErrorContext(ctx context.Context, msg string, args ...any)
}
