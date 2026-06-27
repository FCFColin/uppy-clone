package requestctx

import "context"

type proxyKey struct{}

// WithTrustedProxy marks whether the request peer is a trusted reverse proxy.
func WithTrustedProxy(ctx context.Context, trusted bool) context.Context {
	return context.WithValue(ctx, proxyKey{}, trusted)
}

// IsTrustedProxy reports whether X-Forwarded-* headers were honored for this request.
func IsTrustedProxy(ctx context.Context) bool {
	v, ok := ctx.Value(proxyKey{}).(bool)
	return ok && v
}
