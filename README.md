# 印章生成平台 V2

可运行的 React + TypeScript + Go 印章编辑与商业导出平台，覆盖产品文档定义的 M1–M4。

## 已实现

- 完整 SealConfig v2 编辑器：圆/椭圆/方章、六类文字/中心图片图层、边框高级参数、五种确定性纹理、模板、本地草稿与 40 步撤销重做。
- 本地 SVG/PNG 预览导出；Go 严格校验并渲染正式 SVG；Node Worker 使用 resvg + sharp 生成 PNG、重编码上传图片并移除元数据。
- 注册/登录、PBKDF2 密码哈希、持久化 HttpOnly 会话、设备查看/吊销、管理员 TOTP。
- 云配置、异步生成历史、幂等任务、私有对象存储、120 秒单次下载令牌。
- 固定服务端价格、支付回调 HMAC 验签/金额校验/幂等、单次解锁、VIP、退款与发票流程。
- 用户/订单/任务/退款/发票/模板/字体/纹理/审计后台；VIP 资源配置按服务端权益过滤。
- PostgreSQL 业务状态、Redis 分布式队列与路由限流；无外部依赖时自动使用原子 JSON 文件和本地队列。
- Origin/CSRF 防护、CORS 白名单、CSP 上报、上传真实类型/大小/像素/重编码校验、对象级鉴权、API no-store、Prometheus 指标。

## 本地开发

```bash
cp .env.example .env
make test
make dev-api       # http://localhost:8080
make dev-worker    # http://localhost:8090
make dev-web       # http://localhost:5173
```

不设置 `APP_DATABASE_URL` / `APP_REDIS_URL` 时，API 使用 `apps/api/tmp/data` 下的本地持久化，适合零依赖开发。

## 完整容器栈

```bash
docker compose up -d --build
BASE_URL=http://localhost:8080 ./scripts/smoke-test.sh
```

Web 入口为 `http://localhost:8088`。Compose 会启动 PostgreSQL、迁移、Redis、API、PNG Worker 和 Nginx Web。

生产环境必须设置强随机 `APP_PAYMENT_CALLBACK_SECRET`、Base32 `APP_ADMIN_MFA_SECRET`，关闭支付模拟并由 TLS 入口代理服务。部署与恢复步骤见 [deployment.md](docs/operations/deployment.md)，上线门禁见 [acceptance.md](docs/operations/acceptance.md)，接口契约见 [openapi.yaml](docs/contracts/openapi.yaml)。

## 安全边界

浏览器弱阻吓不是安全边界。无水印权限、价格、字体/资源许可、上传处理、正式渲染、私有文件与下载消费均由后端重新计算。完整商业字体不得放入 `apps/web/public` 或任何公开静态目录。
