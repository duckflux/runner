package engine

import "context"

type baseCWDKey struct{}

// WithBaseCWD stores the base working directory used by exec participants when
// neither participant.cwd nor defaults.cwd is defined.
func WithBaseCWD(ctx context.Context, cwd string) context.Context {
	return context.WithValue(ctx, baseCWDKey{}, cwd)
}

func baseCWDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(baseCWDKey{}).(string)
	return v
}
