package device

import "context"

// context keys for device operations.
type ctxKey int

const (
	forceKey ctxKey = iota
	allowOverwriteKey
	yesIKnowKey
	ptSigKey
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

// WithPartitionSignatures returns a context carrying expected partition table
// signatures. Empty strings skip the comparison.
func WithPartitionSignatures(ctx context.Context, gpt, mbr string) context.Context {
	return context.WithValue(ctx, ptSigKey, [2]string{gpt, mbr})
}

func partitionSignaturesFromContext(ctx context.Context) (string, string) {
	if v, ok := ctx.Value(ptSigKey).([2]string); ok {
		return v[0], v[1]
	}
	return "", ""
}
