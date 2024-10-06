package monitor

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"code.gopub.tech/commons/choose"
	"code.gopub.tech/commons/iters"
	"code.gopub.tech/commons/nums"
	"code.gopub.tech/commons/syncs"
	"code.gopub.tech/commons/values"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
)

// https://www.robustperception.io/how-does-a-prometheus-counter-work/
// https://www.robustperception.io/how-does-a-prometheus-gauge-work/
// https://www.robustperception.io/how-does-a-prometheus-summary-work/
// https://www.robustperception.io/how-does-a-prometheus-histogram-work/

// client 是一个监控打点客户端
type client struct {
	namespace   string
	subsystem   string
	names       NameAppends
	registry    *prometheus.Registry
	constLabels map[string]string
	logger      func(context.Context, string, ...any)
	buckets     []float64
	objectives  map[float64]float64
	counter     *syncs.Map[string, *prometheus.CounterVec]
	gauge       *syncs.Map[string, *prometheus.GaugeVec]
	histogram   *syncs.Map[string, values.Tuple2[*prometheus.HistogramVec, prometheus.HistogramOpts]]
	summary     *syncs.Map[string, values.Tuple2[*prometheus.SummaryVec, prometheus.SummaryOpts]]
}

// NameAppends 自定义 Counter/Gauge/Histogram/Summary 指标名称前缀/后缀
type NameAppends struct {
	Counter   NameAppend
	Gauge     NameAppend
	Timer     NameAppend
	Histogram NameAppend
	Summary   NameAppend
}

// NameAppend 自定义指标名称前缀/后缀
// 只能使用英文字母、数字、下划线、冒号 [a-zA-Z0-9_:]
type NameAppend struct {
	Prefix string
	Suffix string
}

// NewClient 新建监控打点客户端
func NewClient(opts ...Opt) *client {
	c := &client{
		counter:   syncs.NewMap[string, *prometheus.CounterVec](),
		gauge:     syncs.NewMap[string, *prometheus.GaugeVec](),
		histogram: syncs.NewMap[string, values.Tuple2[*prometheus.HistogramVec, prometheus.HistogramOpts]](),
		summary:   syncs.NewMap[string, values.Tuple2[*prometheus.SummaryVec, prometheus.SummaryOpts]](),
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.names == (NameAppends{}) {
		c.names = NameAppends{
			Counter:   NameAppend{Prefix: "counter:"},
			Gauge:     NameAppend{Prefix: "gauge:"},
			Timer:     NameAppend{Prefix: "timer:", Suffix: "_seconds"},
			Histogram: NameAppend{Prefix: "histogram:"},
			Summary:   NameAppend{Prefix: "summary:"},
		}
	}
	if c.registry == nil {
		c.registry = prometheus.NewRegistry()
	}
	if c.constLabels == nil {
		c.constLabels = map[string]string{}
	}
	if c.logger == nil {
		c.logger = slog.WarnContext
	}
	if len(c.buckets) == 0 {
		_ = prometheus.DefBuckets
		c.buckets = []float64{ // prometheus.DefBuckets
			.005, // 5ms
			.01,  // 10ms
			.025, // 25ms
			.05,  // 50ms
			.1,   // 100ms
			.25,  // 250ms
			.5,   // 500ms
			1,    // 1s
			2.5,  // 2.5s
			5,    // 5s
			10,   // 10s
		}
	}
	if len(c.objectives) == 0 {
		c.objectives = map[float64]float64{}
	}
	return c
}

// EscapeName 对指标名转义
//
// 满足 `^[a-zA-Z_:][a-zA-Z0-9_:]*$` 的直接返回,
// 否则会进行转义: `U__` 开头, 不在上述范围的字符转义为 `_unicode_` 编码.
func EscapeName(s string) string {
	return model.EscapeName(s, model.NameEscapingScheme)
}

type Opt func(*client)

// WithNamespace 统一设置指标的名称空间
// 默认值为空字符串
func WithNamespace(name string) Opt {
	return func(c *client) {
		c.namespace = EscapeName(name)
	}
}

// WithSubsystem 统一设置指标名所属子系统
// 默认值为空字符串
func WithSubsystem(name string) Opt {
	return func(c *client) {
		c.subsystem = EscapeName(name)
	}
}

// WithNameAppend 统一设置指标名前缀/后缀
// 前缀默认值分别是 "counter:" "gauge:" "histogram:" "summary:"
// 后缀默认值是空字符串
func WithNameAppend(nameAppend NameAppends) Opt {
	return func(c *client) {
		c.names = nameAppend
	}
}

// WithRegistry 使用指定的 registry
// 默认值是 nil 会通过 `prometheus.NewRegistry()` 生成一个
func WithRegistry(registry *prometheus.Registry) Opt {
	return func(c *client) {
		c.registry = registry
	}
}

// WithConstLabels 统一设置指标常量标签
// 默认值是空的 map
func WithConstLabels(labels map[string]string) Opt {
	return func(c *client) {
		c.constLabels = labels
	}
}

// WithLogger 设置日志输出
// 默认值是 [slog.WarnContext]
func WithLogger(logger func(context.Context, string, ...any)) Opt {
	return func(c *client) {
		c.logger = logger
	}
}

// WithBuckets 设置 histogram 类型指标值的默认分布
// 默认值是 [prometheus.DefBuckets]
func WithBuckets(buckets []float64) Opt {
	return func(c *client) {
		c.buckets = buckets
	}
}

// WithObjectives 设置 summary 类型指标值的默认分位数
// 默认值是空的 map, 表示不使用 summary 记录分位数.
// (因为客户端计算分位数性能不高, 且不能用于聚合)
func WithObjectives(objectives map[float64]float64) Opt {
	return func(c *client) {
		c.objectives = objectives
	}
}

// Handler 返回一个 http.Handler 用于提供 prometheus 指标数据
func (c *client) Handler() http.Handler {
	return promhttp.InstrumentMetricHandler(c.registry,
		promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{
			Registry: c.registry,
		}))
}

// Registry 返回客户端使用的 registry
func (c *client) Registry() *prometheus.Registry {
	return c.registry
}

// Record 记录打点 累加计数器 +1
//
//	// namespace:subsystem:counter:xxx_throughput
//	c.Record(ctx, "xxx_throughput", "打点计数说明")
func (c *client) Record(ctx context.Context, name, desc string, kvs ...string) {
	c.RecordN(ctx, name, desc, 1, kvs...)
}

// RecordN 记录打点 累加计数器 +n
//
//	// namespace:subsystem:counter:xxx_throughput
//	c.RecordN(ctx, "xxx_throughput", "打点计数说明", 10)
func (c *client) RecordN(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	opt := c.prometheusOpt(name, desc, c.names.Counter)
	keys, tags := tags(ctx, kvs...)
	v := c.getCounter(ctx, opt, keys)
	m, err := v.GetMetricWith(tags)
	if err != nil {
		c.recordErr(opt.Name, "get_counter")
		c.logger(ctx, "get_counter|GetMetricWithLabelFailed", "name", opt.Name, "help", desc, "err", err)
		return
	}
	m.Add(nums.To[float64](value))
}

func (c *client) prometheusOpt(name, desc string, na NameAppend) prometheus.Opts {
	return prometheus.Opts{
		Name:        c.buildFQName(name, na),
		Help:        desc,
		ConstLabels: c.constLabels,
	}
}

func (c *client) buildFQName(name string, na NameAppend) string {
	name = na.Prefix + EscapeName(name) + na.Suffix
	names := iters.Of(c.namespace, c.subsystem, name).
		Filter(values.IsNotZero).
		ToSlice()
	return strings.Join(names, ":")
}

func tags(ctx context.Context, kvs ...string) (keys []string, tags map[string]string) {
	tags = CtxGetLabels(ctx) // 获取 ctx 中的 label
	rangeKV(kvs, func(k, v string) {
		tags[k] = v // 添加传入的 kv, 可能覆盖 ctx 中的
	})
	keys = iters.ToSlice(maps.Keys(tags))
	return
}

func (c *client) getCounter(ctx context.Context, o prometheus.Opts, labels []string) *prometheus.CounterVec {
	opt := prometheus.CounterOpts(o)
	v, loaded := c.counter.LoadOrStore(opt.Name, prometheus.NewCounterVec(opt, labels))
	if !loaded {
		c.register(ctx, v, o.Name, o.Help, "register_counter")
	}
	return v
}

func (c *client) register(ctx context.Context, m prometheus.Collector, name, help, kind string) {
	if err := c.registry.Register(m); err != nil && c.logger != nil {
		dup := isAlreadyRegisteredError(err)
		errKind := kind + choose.If(dup, "_dup", "")
		c.logger(ctx, errKind, "name", name, "help", help, "err", err)
		c.recordErr(name, errKind)
	}
}

func isAlreadyRegisteredError(err error) bool {
	are := &prometheus.AlreadyRegisteredError{}
	return errors.As(err, are)
}

func (c *client) recordErr(name, kind string) {
	opt := c.prometheusOpt("internal_monitor_error", "打点异常", c.names.Counter)
	v := prometheus.NewCounterVec(prometheus.CounterOpts(opt), []string{
		"name",
		"kind",
	})
	m, _ := v.GetMetricWith(prometheus.Labels{
		"kind": kind,
		"name": name,
	})
	c.registry.Register(m)
	m.Inc()
}

// Store 存储当前瞬时值
//
//	// namespace:subsystem:gauge:current_goroutinue_num
//	c.Store(ctx, "current_goroutinue_num", "指标含义", 10)
func (c *client) Store(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	opt := c.prometheusOpt(name, desc, c.names.Gauge)
	keys, tags := tags(ctx, kvs...)
	v := c.getGauge(ctx, opt, keys)
	m, err := v.GetMetricWith(tags)
	if err != nil {
		c.recordErr(opt.Name, "get_gauge")
		c.logger(ctx, "get_gauge|GetMetricWithLabelFailed", "name", opt.Name, "help", desc, "err", err)
		return
	}
	m.Set(nums.To[float64](value))
}

func (c *client) getGauge(ctx context.Context, o prometheus.Opts, labels []string) *prometheus.GaugeVec {
	opt := prometheus.GaugeOpts(o)
	v, loaded := c.gauge.LoadOrStore(opt.Name, prometheus.NewGaugeVec(opt, labels))
	if !loaded {
		c.register(ctx, v, o.Name, o.Help, "register_gauge")
	}
	return v
}

// Cost 记录耗时(使用 Timer 指标前缀/后缀)
//
//	start := time.Now()
//	// do something
//	// namespace:subsystem:timer:some_thing_cost_seconds_bucket
//	// namespace:subsystem:timer:some_thing_cost_seconds_sum
//	// namespace:subsystem:timer:some_thing_cost_seconds_count
//	c.Cost(ctx, "some_thing_cost", "打点说明", time.Since(start))
func (c *client) Cost(ctx context.Context, name, desc string, cost time.Duration, kvs ...string) {
	opt := c.prometheusOpt(name, desc, c.names.Timer)
	c.recordHistogram(ctx, opt, cost.Seconds(), c.buckets, kvs...)
}

// CostBuckets 记录耗时(自定义耗时分布)(使用 Timer 指标前缀/后缀)
//
//	start := time.Now()
//	// do something
//	// namespace:subsystem:timer:some_thing_cost_seconds_bucket
//	// namespace:subsystem:timer:some_thing_cost_seconds_sum
//	// namespace:subsystem:timer:some_thing_cost_seconds_count
//	c.CostBuckets(ctx, "some_thing_cost", "打点说明", time.Since(start), []float64{1, 2, 3})
func (c *client) CostBuckets(ctx context.Context, name, desc string, cost time.Duration, buckets []time.Duration, kvs ...string) {
	secondsBucket := iters.Maps(iters.Of(buckets...), func(d time.Duration) float64 { return d.Seconds() })
	opt := c.prometheusOpt(name, desc, c.names.Timer)
	c.recordHistogram(ctx, opt, cost.Seconds(), secondsBucket.ToSlice(), kvs...)
}

func (c *client) recordHistogram(ctx context.Context, opt prometheus.Opts, value nums.AnyNumber, buckets []float64, kvs ...string) {
	keys, tags := tags(ctx, kvs...)
	v := c.getHistogram(ctx, prometheus.HistogramOpts{
		Name:        opt.Name,
		Help:        opt.Help,
		ConstLabels: opt.ConstLabels,
		Buckets:     buckets,
	}, keys)
	if !slices.Equal(v.Val2.Buckets, buckets) {
		c.recordErr(opt.Name, "histogram_buckets_mismatch")
		c.logger(ctx, "histogram_buckets_mismatch",
			"name", opt.Name, "help", opt.Help,
			"wantBucket", buckets, "actual", v.Val2.Buckets,
		)
	}
	m, err := v.Val1.GetMetricWith(tags)
	if err != nil {
		c.recordErr(opt.Name, "get_histogram")
		c.logger(ctx, "get_histogram|GetMetricWithLabelFailed", "name", opt.Name, "help", opt.Help, "err", err)
		return
	}
	m.Observe(nums.To[float64](value))
}

func (c *client) getHistogram(
	ctx context.Context, opt prometheus.HistogramOpts, labels []string,
) values.Tuple2[*prometheus.HistogramVec, prometheus.HistogramOpts] {
	v, loaded := c.histogram.LoadOrStore(opt.Name, values.Make2(prometheus.NewHistogramVec(opt, labels), opt))
	if !loaded {
		c.register(ctx, v.Val1, opt.Name, opt.Help, "register_histogram")
	}
	return v
}

// Timer 记录耗时(使用 Timer 指标前缀/后缀)
//
// @param buckets 如果不传则使用构造 client 时指定的默认值(如果未指定则使用 [prometheus.DefBuckets])
// 如果不需要记录耗时分布, 请使用 Observe
//
//	timer := c.Timer()
//	// do something
//	cost := timer(ctx, "some_thing_cost", "指标描述") // 返回耗时
//
//	defer c.Timer()(ctx, "some_thing_cost", "打点说明") // 也可以忽略返回值
//	defer c.Timer(.1, .5, 1, 5, 10)(ctx, "some_thing_cost", "打点说明") // 自定义耗时分布
//
//	// namespace:subsystem:timer:some_thing_cost_seconds_bucket
//	// namespace:subsystem:timer:some_thing_cost_seconds_sum
//	// namespace:subsystem:timer:some_thing_cost_seconds_count
func (c *client) Timer(buckets ...float64) func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
	if len(buckets) == 0 {
		buckets = c.buckets
	}
	start := time.Now()
	return func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
		cost := time.Since(start)
		opt := c.prometheusOpt(name, desc, c.names.Timer)
		c.recordHistogram(ctx, opt, cost.Seconds(), buckets, kvs...)
		return cost
	}
}

// Histogram 记录值的分布. 如果无需记录分布, 请使用 Summary
// (使用 Histogram 指标前缀/后缀)
//
//	// namespace:subsystem:histogram:some_thing_cost_bucket
//	// namespace:subsystem:histogram:some_thing_cost_sum
//	// namespace:subsystem:histogram:some_thing_cost_count
//	c.Histogram(ctx, "some_thing_cost", "打点说明", 1.5, []float64{1, 2, 3})
func (c *client) Histogram(ctx context.Context, name, desc string, value nums.AnyNumber, buckets []float64, kvs ...string) {
	opt := c.prometheusOpt(name, desc, c.names.Histogram)
	c.recordHistogram(ctx, opt, value, buckets, kvs...)
}

// Observe 记录耗时摘要(使用 Timer 指标前缀/后缀)
//
//	// namespace:subsystem:timer:some_thing_cost_seconds_sum
//	// namespace:subsystem:timer:some_thing_cost_seconds_count
//	defer c.Observe()(ctx, "some_thing_cost", "打点说明")
func (c *client) Observe() func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
	start := time.Now()
	return func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
		cost := time.Since(start)
		opt := c.prometheusOpt(name, desc, c.names.Timer)
		c.recordSummary(ctx, opt, cost.Seconds(), nil, kvs...)
		return cost
	}
}

// Summary 记录摘要(使用 Summary 指标前缀/后缀)
//
// 使用 client 构造时指定的分位数(默认值是 nil, 表示无需记录分位数, 此时 _sum, _count 指标仍然可用),
// 不建议在客户端指定分位数, 因为客户端计算分位数性能不高, 且不能用于聚合;
// 如果需要分位数, 建议使用 Histogram 记录, 并使用 `histogram_quantile` 查询分位数.
//
//	// namespace:subsystem:summary:some_thing_cost_sum
//	// namespace:subsystem:summary:some_thing_cost_count
//	c.Summary(ctx, "some_thing_cost", "打点说明", 1.5)
func (c *client) Summary(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	c.SummaryObjectives(ctx, name, desc, value, c.objectives, kvs...)
}

// SummaryObjectives 记录摘要(自定义分位数)(使用 Summary 指标前缀/后缀)
//
// 不建议使用, 因为客户端计算分位数性能不高, 且不能用于聚合
//
//	// namespace:subsystem:summary:some_thing_cost{quantile="0.5"}
//	// namespace:subsystem:summary:some_thing_cost{quantile="0.9"}
//	// namespace:subsystem:summary:some_thing_cost_sum
//	// namespace:subsystem:summary:some_thing_cost_count
//	c.SummaryObjectives(ctx, "some_thing_cost", "打点说明", 1.5, map[float64]float64{0.5: 0.05, 0.9: 0.01})
func (c *client) SummaryObjectives(ctx context.Context, name, desc string, value nums.AnyNumber, objectives map[float64]float64, kvs ...string) {
	opt := c.prometheusOpt(name, desc, c.names.Summary)
	c.recordSummary(ctx, opt, value, objectives, kvs...)
}

func (c *client) recordSummary(ctx context.Context, opt prometheus.Opts, value nums.AnyNumber, objectives map[float64]float64, kvs ...string) {
	keys, tags := tags(ctx, kvs...)
	v := c.getSummary(ctx, prometheus.SummaryOpts{
		Name:        opt.Name,
		Help:        opt.Help,
		ConstLabels: opt.ConstLabels,
		Objectives:  objectives,
	}, keys)
	if !maps.Equal(v.Val2.Objectives, objectives) {
		c.recordErr(opt.Name, "summary_objectives_mismatch")
		c.logger(ctx, "summary_objectives_mismatch",
			"name", opt.Name, "help", opt.Help,
			"wantObjectives", objectives, "actual", v.Val2.Objectives,
		)
	}
	m, err := v.Val1.GetMetricWith(tags)
	if err != nil {
		c.recordErr(opt.Name, "get_summary")
		c.logger(ctx, "get_summary|GetMetricWithLabelFailed", "name", opt.Name, "help", opt.Help, "err", err)
		return
	}
	m.Observe(nums.To[float64](value))
}

func (c *client) getSummary(ctx context.Context, opt prometheus.SummaryOpts, labels []string) values.Tuple2[*prometheus.SummaryVec, prometheus.SummaryOpts] {
	v, loaded := c.summary.LoadOrStore(opt.Name, values.Make2(prometheus.NewSummaryVec(opt, labels), opt))
	if !loaded {
		c.register(ctx, v.Val1, opt.Name, opt.Help, "register_summary")
	}
	return v
}
