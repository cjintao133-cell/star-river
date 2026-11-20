# 许可

## 总则
- **本仓库整体采用 [Apache License 2.0](../LICENSE)。** 包括全部 `kernel/*` 包、
  `native/`、`cmd/`、`scripts/`、`.github/`、以及记忆/情绪/主动等「能力插件包」
  （`kernel/hippocampus`、`kernel/endocrine`、`kernel/proactive`）。
- 无「闭源区」。历史上曾设想的"三包闭源"方案**已废弃**，改为全开源。
- Apache-2.0 含专利授权、商标条款与保留声明，生态友好、企业可采用。

## 为什么全开源
- 记忆曲线、情绪维度、向量召回等均为公开常识，闭源无实际壁垒，反而阻碍采用与贡献。
- 框架的价值在"被采用、被信任、可持续维护"，不在"藏一段代码"。

## 分层约束（工程规矩，非许可证）
虽然全部开源，仍**保留依赖方向约束**（源码整洁 + 可拔插）：
- **底座包（`kernel/bus`、`kernel/adapter`、`kernel/store`、`kernel/orchestrator`、
  `kernel/tool`、`kernel/core/*` 等）不得 import 能力插件包**
  （`hippocampus` / `endocrine` / `proactive`）。
- 能力插件包可以 import 底座。
- 该约束由 `scripts/check-open-split.sh` 持续校验（底座可单独编译、且不依赖插件包）。

## 贡献即同意
按 Apache-2.0，贡献默认以同一许可授权（inbound = outbound）。
