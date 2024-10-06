package monitor

import (
	"context"
	"net/http"
	"time"

	"code.gopub.tech/commons/nums"
)

// PATTERN_METRICS 默认的 metrics 路径
const PATTERN_METRICS = "/metrics"

var defaultClient = NewClient()

// 默认往 http.DefaultServeMux 上注册
// 如果应用程序不使用 http.DefaultServeMux, 则需要使用 HTTPHandler() 自行注册
func init() {
	http.Handle(PATTERN_METRICS, HTTPHandler())
}

// HTTPHandler 返回一个 http.Handler 用于暴露 metrics 数据
func HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// defaultClient 可能被修改
		// 因此不直接使用 defaultClient.Handler()
		// 而是在这里实时获取
		defaultClient.Handler().ServeHTTP(w, r)
	})
}

// Default 获取全局默认的 client
func Default() *client {
	return defaultClient
}

// SetDefault 设置全局默认的 client
func SetDefault(d *client) {
	defaultClient = d
}

// Record 记录打点 累加计数器 +1
func Record(ctx context.Context, name, desc string, kvs ...string) {
	defaultClient.Record(ctx, name, desc, kvs...)
}

// RecordN 记录打点 累加计数器 +n
func RecordN(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	defaultClient.RecordN(ctx, name, desc, value, kvs...)
}

// Store 存储当前瞬时值
func Store(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	defaultClient.Store(ctx, name, desc, value, kvs...)
}

// Cost 记录耗时(使用 Timer 指标前缀/后缀)
func Cost(ctx context.Context, name, desc string, cost time.Duration, kvs ...string) {
	defaultClient.Cost(ctx, name, desc, cost, kvs...)
}

// CostBuckets 记录耗时分布(使用 Timer 指标前缀/后缀)
func CostBuckets(ctx context.Context, name, desc string, cost time.Duration, buckets []time.Duration, kvs ...string) {
	defaultClient.CostBuckets(ctx, name, desc, cost, buckets, kvs...)
}

// Timer 记录耗时(使用 Timer 指标前缀/后缀)
func Timer() func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
	return defaultClient.Timer()
}

// Histogram 记录值的分布. 如果无需记录分布, 请使用 Summary
func Histogram(ctx context.Context, name, desc string, value nums.AnyNumber, buckets []float64, kvs ...string) {
	defaultClient.Histogram(ctx, name, desc, value, buckets, kvs...)
}

// Observe 记录耗时摘要(使用 Timer 指标前缀/后缀)
func Observe() func(ctx context.Context, name, desc string, kvs ...string) time.Duration {
	return defaultClient.Observe()
}

// Summary 记录摘要(使用 Summary 指标前缀/后缀)
func Summary(ctx context.Context, name, desc string, value nums.AnyNumber, kvs ...string) {
	defaultClient.Summary(ctx, name, desc, value, kvs...)
}

// SummaryObjectives 记录摘要(自定义分位数)(使用 Summary 指标前缀/后缀)
func SummaryObjectives(ctx context.Context, name, desc string, value nums.AnyNumber, objectives map[float64]float64, kvs ...string) {
	defaultClient.SummaryObjectives(ctx, name, desc, value, objectives, kvs...)
}
