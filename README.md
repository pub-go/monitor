# monitor
metrics client for prometheus

```bash
go get code.gopub.tech/monitor@latest
```

```go
monitor.Record(ctx, "xxx_throughput", "xxx总量")
monitor.RecordN
monitor.Store
monitor.Cost
defer monitor.Timer()()
monitor.Histogram
defer monitor.Observe()()
monitor.Summary
monitor.SummaryObjectives
```
