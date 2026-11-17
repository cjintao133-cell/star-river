# 许可与开源/闭源边界

## 总则
- 仓库根 LICENSE 为 Apache License 2.0，**仅适用于开源区**。
- 闭源区不适用 Apache-2.0，保留所有权利（All Rights Reserved），以各包 README 声明为准。
- 边界依据：《蓝图评审与优化版》冲突修正清单 #4（docs/constraints-12.md）。

## 开源区（Apache-2.0）
- kernel/bus（总线骨架）
- kernel/adapter（三协议适配器）
- kernel/cards（角色卡导入解析器）
- kernel/tool 中的 MCP SDK 部分（T-M6-01 实现时细分）
- native/ 中通用 C 模块（device_probe 等；向量检索模块归属 T-M3-01 时再定，默认随 hippocampus 闭源）
- scripts/、.github/（构建脚本与 CI）
- ui-android / ui-desktop：蓝图判定"UI 可开源"，v1 期间随仓库私有，公开发布时再定许可，不视为承诺。

## 闭源区（All Rights Reserved）
- kernel/hippocampus（记忆引擎）
- kernel/endocrine（情绪引擎）
- kernel/proactive（主动调度）

## 其他
- kernel/orchestrator、kernel/store、kernel/visual、svc-dist：边界未在蓝图明列，暂随仓库私有、未定许可，拆分（T-M7-05）前由主人裁决。
- kernel/codebrain：v1 仅 JSON 配置生成（清单 #9），真实代码生成属 v2，不进 v1 验收。
- 硬规则：开源区代码不得 import 闭源包（红线 2）；闭源包可以 import 开源区。
