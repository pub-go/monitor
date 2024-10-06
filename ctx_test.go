package monitor_test

import (
	"testing"

	"code.gopub.tech/commons/assert"
	"code.gopub.tech/monitor"
)

func TestCtx(t *testing.T) {
	ctx := monitor.CtxAddLabels(ctx)
	m := monitor.CtxGetLabels(ctx)
	assert.True(t, len(m) == 0)

	ctx = monitor.CtxAddLabels(ctx, "k1", "v2", "k2")
	m = monitor.CtxGetLabels(ctx)
	assert.DeepEqual(t, m, map[string]string{"k1": "v2"})
}
