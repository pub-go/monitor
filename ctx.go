package monitor

import (
	"context"
	"maps"
)

type ctxKey struct{}

// CtxAddLabels 往 ctx 中添加 labels, 返回新的 ctx
func CtxAddLabels(ctx context.Context, kvs ...string) context.Context {
	var m = CtxGetLabels(ctx)
	rangeKV(kvs, func(k, v string) {
		m[k] = v
	})
	ctx = context.WithValue(ctx, ctxKey{}, m)
	return ctx
}

// CtxGetLabels 从 ctx 中获取 labels
func CtxGetLabels(ctx context.Context) map[string]string {
	m := ctxGetLabels(ctx)
	return maps.Clone(m)
}

func ctxGetLabels(ctx context.Context) map[string]string {
	v := ctx.Value(ctxKey{})
	if v != nil {
		return v.(map[string]string)
	} else {
		return map[string]string{}
	}
}

func rangeKV(kvs []string, f func(string, string)) {
	size := len(kvs)
	for i := 0; i < size-1; i += 2 {
		f(kvs[i], kvs[i+1])
	}
}
