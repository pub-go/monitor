/*
package monitor provides a simple way to monitor your application.

一个 prometheus 的简易封装, 用于监控打点.

# Init 初始化

使用默认的全局客户端, 无需初始化, 如需自定义参数可使用 `NewClient` 初始化.
支持自定义的参数有:

	WithNamespace("namespace")	// 默认值为空
	WithSubsystem("subsystem")	// 默认值为空
	// 指标类型不同, 默认值不同
	// Counter 指标, 前缀默认值是 `counter:`
	// Gauge 指标, 前缀默认值是 `gauge:`
	// Timer 指标, 前缀默认值是 `timer:`, 后缀默认值是 `_seconds`
	// Histogram 指标, 前缀默认值是 `histogram:`
	// Summary 指标, 前缀默认值是 `summary:`
	WithNameAppend(monitor.NameAppends{})
	WithRegistry(registry)
	WithConstLabels(map[string]string{})
	WithLogger(func(context.Context, string, ...any))
	// 默认值是 prometheus.DefBuckets
	// .005/5ms, .01/10ms, .025/25ms, .05/50ms, .1/100ms,
	// .25/250ms, .5/500ms, 1/1s, 2.5/2.5s, 5/5s, 10/10s.
	WithBuckets([]float64{})
	WithObjectives(map[float64]float64{})

应用程序使用 Record(ctx, "name", "help") 等 API 进行打点, 
指标名会自动拼接前缀/后缀, 然后再附加上名称空间/子模块, 最终格式为:

	<namespace>:<subsystem>:<prefix><name><suffix>{<labels>}

默认会在 /metrics 端点暴露打点数据.
也可以通过 HTTPHandler() 获取 handler 自行注册到不同的路径.

# API 使用

Counter 类型指标用于计数, 只能增加, 不能减少(除非程序重启).
指标名默认会拼接 `counter:` 前缀.

	monitor.Record(ctx, name, help)
	monitor.RecordN(ctx, name, help, value)

Gauge 类型指标用于存储当前瞬时值, 可以增加, 减少, 重置.
指标名默认会拼接 `gauge:` 前缀.

	monitor.Store(ctx, name, help, value)

记录耗时, 使用 Cost, CostBuckets, 或 Timer/Observe  方法.
指标名默认会拼接 `timer:` 前缀, `_seconds` 后缀.

	// cost 是 time.Duration 类型
	// 会自动通过 Seconds 方法记录为秒值(因此时间类指标默认后缀是 _seconds)
	monitor.Cost(ctx, name, help, cost)
	// 如需自定义耗时分布, 可以使用 buckets 参数
	monitor.CostBuckets(ctx, name, help, cost, buckets)
	// 使用 Timer 方法配合 defer 使用, 会自动计时
	defer monitor.Timer(buckets...)(ctx, name, help)
	// 使用 Observe 方法配合 defer 使用, 会自动记录耗时
	// Observe 方法底层使用 Summary 类型
	defer monitor.Observe()(ctx, name, help)

记录直方图
指标名默认会拼接 `histogram:` 前缀.

	monitor.Histogram(ctx, name, help, value, buckets)

记录摘要
指标名默认会拼接 `summary:` 前缀.

	monitor.Summary(ctx, name, help, value)
	monitor.SummaryObjectives(ctx, name, help, value, objectives)

# 标签 Labels

每个 API 都可选传入标签(labels), 
一个指标一旦附加了标签, 后续每次打点都需要附加相同的标签名(label name),
标签值(label value)可以不同. 

	monitor.Record(ctx, "name", "help", "k1", "v1", "k2", "v2")
	monitor.Record(ctx, "name", "help", "k1", "v1", "k2", "v22")
	// no effect, missing k2 label 缺少标签的打点会被忽略
	monitor.Record(ctx, "name", "help", "k1", "v1")
	// no effect, mismatch labels 标签对不同的打点会被忽略
	monitor.Record(ctx, "name", "help", "k1", "v1", "x", "y")

如果标签对较多, 可以使用 CtxAddLabels 方法往 ctx 上附加, 后续每次打点会自动解析

	ctx = monitor.CtxAddLabels(ctx, "k1", "v1", "k2", "v2")
	monitor.Record(ctx, "name", "help")

*/
package monitor
