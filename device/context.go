package device

import "context"

// context keys for device operations.
type ctxKey int

const (
	forceKey ctxKey = iota
	allowOverwriteKey
	yesIKnowKey
)

// WithForce returns a context that carries the --force flag state.
func WithForce(ctx context.Context, force bool) context.Context {
	return context.WithValue(ctx, forceKey, force)
}

func forceFromContext(ctx context.Context) bool {
	if v, ok := ctx.Value(forceKey).(bool); ok {
		return v
	}
	return false
}

// WithAllowOverwrite returns a context that carries the --allow-overwrite flag state.
func WithAllowOverwrite(ctx context.Context, allow bool) context.Context {
	return context.WithValue(ctx, allowOverwriteKey, allow)
}

func allowOverwriteFromContext(ctx context.Context) bool {
	if v, ok := ctx.Value(allowOverwriteKey).(bool); ok {
		return v
	}
	return false
}

// WithYesIKnow returns a context that carries the --yes-i-know flag state.
func WithYesIKnow(ctx context.Context, yes bool) context.Context {
	return context.WithValue(ctx, yesIKnowKey, yes)
}

func yesIKnowFromContext(ctx context.Context) bool {
	if v, ok := ctx.Value(yesIKnowKey).(bool); ok {
		return v
	}
	return false
}
