package monitor_test

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"code.gopub.tech/commons/arg"
	"code.gopub.tech/monitor"
	"github.com/prometheus/client_golang/prometheus"
)

var ctx = context.Background()

func TestMonitor(t *testing.T) {
	start := time.Now()
	c := monitor.NewClient(
		monitor.WithNamespace("namespace"),
		monitor.WithSubsystem("subsystem"),
		monitor.WithNameAppend(monitor.NameAppends{}),
		monitor.WithRegistry(nil),
		monitor.WithConstLabels(map[string]string{
			"host": "host_name",
		}),
		monitor.WithLogger(nil),
		monitor.WithBuckets(nil),
		monitor.WithObjectives(nil),
	)
	registry := c.Registry()

	printMetricPage := func() {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		c.Handler().ServeHTTP(w, req)
		resp := w.Result()
		body, err := io.ReadAll(resp.Body)
		t.Logf("err=%+v, body=%s", err, body)
	}
	printMetricPage()

	bg := context.Background()
	// 往 ctx 上添加 label
	ctx := monitor.CtxAddLabels(ctx, "k", "v")

	// xxx_throughput{k="v",x="y1"}
	c.Record(ctx, "xxx_throughput", "xxx总量", "x", "y1")
	// 使用不带 label 的 ctx, 需要保证 label name 和已注册的指标一致
	// xxx_throughput{k="v2",x="y2"}
	c.Record(bg, "xxx_throughput", "xxx总量", "k", "v2", "x", "y2")
	// 如果 label name 不一致, 则无法打点, 会打印错误日志
	// get metric failed name=some_name err="label name \"x\" missing in label map"
	c.Record(bg, "xxx_throughput", "xxx总量", "k", "v", "a", "b")

	// 中文指标名会转义
	// U___603b__91cf_{k="v", x="y2"}
	c.RecordN(ctx, "总量", "指标名描述", 2, "x", "y2")

	// Store 瞬时值
	// xxx_current_value{k="v"}
	c.Store(ctx, "xx_current_value", "指标描述", 2)
	// label name 不一致, 无法打点
	c.Store(bg, "xx_current_value", "指标描述", 2)

	time.Sleep(time.Millisecond * 20)
	// xxx_cost_seconds{k="v"}
	c.Cost(ctx, "xxx_cost", "xxx耗时", time.Since(start))
	// label name 不一致, 无法打点
	c.Cost(bg, "xxx_cost", "xxx耗时", time.Since(start))
	c.CostBuckets(ctx, "xxx_cost_detail", "xxx耗时分布", time.Since(start), []time.Duration{
		time.Millisecond * 15,
		time.Millisecond * 20,
		time.Millisecond * 25,
	})
	c.Cost(ctx, "xxx_cost_detail", "xxx耗时", time.Since(start))

	defer c.Timer()(ctx, "my_timer", "timer耗时")
	count := 100
	buckets := prometheus.LinearBuckets(0, 10, 10)
	objectives := map[float64]float64{
		0.5:  0.05,
		0.9:  0.01,
		0.99: 0.001,
	}
	for i := 0; i < count; i++ {
		c.Histogram(ctx, "xxx_histogram", "xxx分布", i, buckets)
		c.Summary(ctx, "xxx_summary", "xxx摘要", i)
		c.SummaryObjectives(ctx, "detail_summary", "xxx摘要", i, objectives)
	}
	defer c.Observe()(ctx, "xxx_observe", "observe耗时")
	// 缺少 labels, 无法打点
	c.Summary(bg, "xxx_summary", "xxx摘要", 50)
	// 该指标之前注册时是不带 objectives 的,
	// 现在传入 objectives 也不会生效, 按无 objectives 打点
	c.SummaryObjectives(ctx, "xxx_summary", "xxx摘要", 50, objectives)

	time.Sleep(time.Millisecond * 10)
	printMetricPage()

	f, err := registry.Gather()
	t.Logf("err=%v, f=%v", err, arg.Indent(f))
}

func TestRegister(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.Register(prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter:xxx_throughput",
	}))
	registry.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gauge:xxx_current_value",
	}))
	registry.Register(prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "histogram:xxx_cost",
	}))
	registry.Register(prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "summary:xxx_cost",
	}))
	c := monitor.NewClient(monitor.WithRegistry(registry))
	c.Record(ctx, "xxx_throughput", "")
	c.Store(ctx, "xxx_current_value", "", 1)
	c.Histogram(ctx, "xxx_cost", "", 1, []float64{.5, 1, 1.5})
	c.Summary(ctx, "xxx_cost", "", 1)
}

func TestEscapeName(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "empty"},
		{name: "abc", args: args{s: "abc"}, want: "abc"},
		{name: "basic", args: args{s: "a_b:c"}, want: "a_b:c"},
		{name: "NumStart", args: args{s: "0a_b:c"}, want: "U___30_a_b:c"},      // U__ 开头, _30_ 表示数字 0
		{name: "utf", args: args{s: "abc 中文"}, want: "U__abc_20__4e2d__6587_"}, // _20_ = 空格, _4e2d_ = 中, _6587_ = 文
		{name: "utf2", args: args{s: "总量"}, want: "U___603b__91cf_"},           // _20_ = 空格, _4e2d_ = 中, _6587_ = 文
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := monitor.EscapeName(tt.args.s); got != tt.want {
				t.Errorf("NormalizeStr() = %v, want %v", got, tt.want)
			}
		})
	}
}
