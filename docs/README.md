# Star River 文档索引

面向"要用 / 要评估 / 要发布这个框架"的人，从这里进：

## 核心三件（先读这三个）
- **[产品说明 product-brief.md](./product-brief.md)** — 它是什么/不是什么、诚实的数据边界、与同类坐标。
- **[使用说明 usage.md](./usage.md)** — 环境、构建、跑通、接云端/接本地(离线)、livecheck、角色卡、嵌入 App。
- **[常见问题 faq.md](./faq.md)** — "到底纯不纯本地"、能不能离线、支持哪些模型、能用于生产吗等。

## 发布前必读
- **[../KNOWN-ISSUES.md](../KNOWN-ISSUES.md)** — 待外部安全评审清单、未闭环红线、真机未验项。**评估/发布前请务必阅读，勿宣称生产级安全。**
- **[../RELEASE.md](../RELEASE.md)** — 发布说明（内容清单、构建、许可）。
- **[privacy-boundary.md](./privacy-boundary.md)** — 数据边界说明书（本地存什么、什么情况出设备）。

## 架构与设计
- **[architecture.md](./architecture.md)** — 包结构、分层、12 条红线、依赖方向。
- **[platform-capability.md](./platform-capability.md)** — 各平台能力对照。
- **[search-design.md](./search-design.md)** — 搜索设计。
- **[secret-storage.md](./secret-storage.md)** — 密钥存储设计。
- **[licensing.md](./licensing.md)** — 许可与开源/闭源边界（**注意：与"全开源"方向存在待统一之处，见 FAQ Q11**）。

## 生态与接入
- **[community-ecosystem-contract.md](./community-ecosystem-contract.md)** — 用 Git 仓库当后端的社区接口契约。
- **[ecosystem-ingestion.md](./ecosystem-ingestion.md)** — 生态接入（把能力导出成 MCP server 等）。
