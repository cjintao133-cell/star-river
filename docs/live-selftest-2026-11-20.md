# 真机级冒烟（硅基流动 SiliconFlow）实测记录（2026-11-20）

> 用真实 key 走真实网络，覆盖年尺度模拟覆盖不到的项：真端点/真模型/真生图/真自拍。
> key 仅落 build/.testkey(600,gitignore)，全程 redact、跑完 shred 销毁；预算上限 ¥5。
> **注意：本文不含任何密钥。**

## 一、端点与模型（自动发现，/models 免费）
- provider「轨迹流动」= **硅基流动 SiliconFlow**（同音），base `https://api.siliconflow.cn/v1`，OpenAI 兼容。
- `/models` 连通，key 有效（LIVECHECK-GREEN）。
- 可用画图模型：`Kwai-Kolors/Kolors`、`Qwen/Qwen-Image`、`Qwen/Qwen-Image-Edit(-2509)` 等；聊天：`Qwen/Qwen2.5-7B-Instruct`、`deepseek-ai/...` 等。

## 二、实测结果（全绿，均带强制水印）
| 项 | 结果 |
|---|---|
| 真聊天 `Qwen/Qwen2.5-7B-Instruct` | **181ms**，回复「收到」，流式正常 |
| 文生图 `Kwai-Kolors/Kolors` | **3.6s**，PNG **1,195,631B**，`HasAIWatermark=true` → build/livecheck-image.png |
| 图生图 `Qwen/Qwen-Image-Edit-2509` | **46.5s**，PNG 629,331B，`HasAIWatermark=true` → build/livecheck-edit.png |
| **真·自拍全链路** `VisualSelfie`(mobilebind) | **26.7s**，PNG **1,176,682B**，`HasAIWatermark=true` → build/livecheck-selfie.png |

自拍全链路 = `InitKernel → CreateCharacter → SetCharacterPersona → VisualInit(backend=siliconflow) → VisualSelfie`，与 App 完全同路径（人设图垫图 + 自拍模板 + 强制水印）。

## 三、实测暴露并修复的真机差异（关键）
- **硅基无 `/images/edits`**：OpenAI 的 multipart 图生图端点 404（实测）。故自拍（垫图 img2img）在硅基上原本必失败。
- 硅基的图生图走 **`/images/generations` + JSON `image` 字段（data URL/base64）**，文生图/图生图共用该端点。
- 修复：新增 `kernel/visual/siliconflow_image.go` `SiliconFlowImageBackend`（文生图/图生图共用 generations，解析 `data`/`images` 两种返回，`netx` 禁重定向）；`visual_bridge.VisualInit` 增 `backend=siliconflow`；App 侧即可用硅基真自拍。
- 红线10 已实证：三条链路输出均 `HasAIWatermark=true`（框架级强制，无关闭 API）。

## 四、工具（可复用）
`cmd/livecheck` 模式：`selftest`(本地水印免费) / `models`(列模型免费) / `chat` / `image`(文生图) / `edit`(图生图) / `selfie`(mobilebind 全链路自拍) / `all`。
- `-backend openai-image|siliconflow`；`-keyfile` 默认 build/.testkey；`-maxreq` 计费硬闸；默认 dry 不联网；全程 redact。

## 五、预算
实耗预计 **< ¥2**（1 次 Kolors + 2 次 Qwen-Image-Edit + 1 次聊天），远低于 ¥5 上限（以硅基账单为准）。

## 六、门禁
`go build/vet/staticcheck ./...` 全绿；`go test ./kernel/...` 全绿；红线2 PASS；**AAR 9,484,232B + APK BUILD SUCCESSFUL 41.6MB**（硅基后端已链入 Android）。

## 七、收尾
测试 key 已 `shred` 销毁（build/.testkey 已删）。App 正式使用走用户自配 BYOK（UI 内 Keystore 持有），不依赖本测试 key。
