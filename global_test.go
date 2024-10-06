package monitor_test

import (
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"code.gopub.tech/commons/assert"
	"code.gopub.tech/commons/times"
	"code.gopub.tech/monitor"
)

func TestMonitorGlobal(t *testing.T) {
	c := monitor.Default()
	assert.NotNil(t, c)
	monitor.SetDefault(c)

	defer func() {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		monitor.HTTPHandler().ServeHTTP(w, req)
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		t.Logf("err=%+v, body=%s", err, body)
	}()

	monitor.Record(ctx, "xxx_throughput", "xxx总量")
	monitor.RecordN(ctx, "xxx_throughput", "xxx总量", 2)
	monitor.Store(ctx, "xxx_current_value", "xxx当前值", 3)

	defer monitor.Timer()(ctx, "xxx_cost", "xxx耗时")

	monitor.Cost(ctx, "xxx_cost", "xxx耗时", time.Millisecond*10)
	buckets := []time.Duration{
		times.Milliseconds(5),
		times.Milliseconds(10),
		times.Milliseconds(15),
	}
	monitor.CostBuckets(ctx, "detail_cost", "耗时分布", time.Millisecond*9, buckets)
	monitor.CostBuckets(ctx, "detail_cost", "耗时分布", time.Millisecond*10, buckets)
	monitor.CostBuckets(ctx, "detail_cost", "耗时分布", time.Millisecond*12, buckets)

	monitor.Histogram(ctx, "xxx_value", "xxx分布", 10, []float64{5, 10, 15})

	defer monitor.Observe()(ctx, "xxx_avg", "xxx平均用时")

	monitor.Summary(ctx, "xxx_avg", "xxx平均用时", 10)
	objectives := map[float64]float64{
		.5:  .05,
		.9:  .01,
		.99: .001,
	}
	monitor.SummaryObjectives(ctx, "detail_summary", "分位数", 10, objectives)
	monitor.SummaryObjectives(ctx, "detail_summary", "分位数", 20, objectives)
	monitor.SummaryObjectives(ctx, "detail_summary", "分位数", 30, objectives)

}
