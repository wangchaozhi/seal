# 上线验收门禁

## 自动化

```bash
make test
cd apps/api && go test -race ./internal/platform ./internal/storage ./internal/httpx
docker compose config --quiet
BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
```

必须全部通过，且 `npm audit --omit=dev` 不存在生产依赖高危漏洞。

## 安全检查

- Web `dist` 无 `.map` 和完整商业字体；前端无支付、数据库、Redis、对象存储密钥。
- 跨用户配置/任务/订单/素材访问返回 404/403；篡改 `vip/paid/watermark` 不提升权限。
- 下载令牌只能消费一次；过期、退款和非所有者均不能访问对象。
- PNG/JPEG/WebP 以真实文件头和解码结果校验，5 MB/2500 万像素限制生效，输出统一重编码 PNG。
- 支付回调的签名、金额和幂等测试通过；生产支付模拟接口为 404。
- 生成、上传、订单、令牌、登录/注册的路由限流生效且 429 包含 `Retry-After`。
- 管理员要求 TOTP，后台变更、退款、发票、封禁均写审计。
- QQ/微信/GitHub/Google OAuth 回调严格校验一次性 state；GitHub/Google 同时使用 PKCE S256；Client Secret 和 access token 不进入前端、日志或持久化用户记录。
- CSP 强制策略启用并能上报；Cookie 为 HttpOnly/SameSite，HTTPS 下为 Secure；跨站修改被 Origin Guard 拒绝。

## 运维检查

- PostgreSQL 备份和对象版本恢复已演练；Redis 清空后 queued/rendering 任务可恢复。
- `/metrics` 已采集请求量、状态与耗时；日志无完整印章文字、密码、支付密钥和原始令牌。
- 告警至少覆盖 5xx、429 激增、队列深度、任务失败率、支付回调异常、磁盘/对象容量。
