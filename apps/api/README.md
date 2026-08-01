# Go API

Go 1.23 API 提供身份会话、云配置、生成队列、订单支付、退款发票、资源后台、审计、上传和私有下载。生产配置使用 pgx/PostgreSQL 与 go-redis；不设置连接 URL 时使用原子 JSON 状态和本地队列。

```bash
go test ./...
go vet ./...
go run ./cmd/server
```

健康检查：`GET /api/v1/health`；Prometheus：`GET /metrics`；正式契约：`../../docs/contracts/openapi.yaml`。
