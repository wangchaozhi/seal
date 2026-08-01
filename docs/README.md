# 文档目录

- `product/`：产品需求、页面流程、功能优先级和验收标准
- `technical/`：Go + React 架构、渲染算法、数据模型和部署建议
- `security/`：F12 弱阻吓、接口防刷、字体与下载防盗、安全验收
- `prototype/`：当前有效、仍作为开发参考的交互原型
- `archive/`：已停止维护的历史需求与原型，仅用于设计追溯
- `contracts/`：OpenAPI、SealConfig JSON Schema、纹理预设和安全策略

工程源码中的实现以当前产品文档和 `contracts/` 为设计基线；`archive/` 中的内容不作为开发或验收依据。若接口或配置字段发生变化，应同步更新 `contracts/`。
