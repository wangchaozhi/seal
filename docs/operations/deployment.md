# 部署、备份与密钥轮换

## 生产前配置

- `APP_ENV=production`，`APP_ENABLE_PAYMENT_SIMULATION=false`。
- `APP_ORIGIN` 必须是唯一 HTTPS Web Origin。
- `APP_PAYMENT_CALLBACK_SECRET` 使用至少 32 字节随机值。
- `APP_ADMIN_MFA_SECRET` 使用认证器可导入的 Base32 随机值；生产配置管理员邮箱时必填。
- PostgreSQL 与 Redis 使用独立账号、TLS/私网访问和持久卷；对象目录挂载私有持久卷。

## 发布

1. 备份 PostgreSQL 与对象卷。
2. 执行 `apps/api/migrations`，迁移均为可重复执行的前向迁移。
3. 构建并扫描镜像，确认 Web 镜像无 `.map`、字体私钥或支付密钥。
4. 先启动 PostgreSQL、Redis、Worker，再启动 API，最后切换 Web/Nginx。
5. 检查 `/api/v1/health`、`/metrics`，运行 `scripts/smoke-test.sh`。

## 备份与恢复

- PostgreSQL：每日全量加持续 WAL/托管 PITR；至少每季度在隔离环境执行恢复演练。
- 对象存储：开启版本和生命周期；数据库快照与对象版本使用同一恢复点标签。
- Redis 队列不是业务事实来源。丢失时 API 启动会扫描 PostgreSQL 中 queued/rendering 任务并重新入队。
- 恢复后抽查用户、订单、任务、对象 SHA-256 与单次令牌状态，再开放流量。

## 密钥轮换

- 支付密钥与 TOTP 秘钥通过密钥管理系统注入，禁止写入镜像或仓库。
- 支付密钥轮换采用渠道双密钥窗口；旧密钥仅在最长回调重试窗口内保留。
- 管理员 TOTP 轮换后吊销该管理员全部设备会话并重新绑定。
- 发生泄漏时同时吊销会话和未消费下载令牌，保留散列审计，不记录原始令牌。

## 回滚

应用镜像可回滚；数据库仅使用向前兼容迁移。若发布失败，恢复上一镜像并暂停产生新写入，禁止直接删除新列/表。
