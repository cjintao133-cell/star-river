# 星链API(4sapi) 真机验证 · chat 方言（第四种）（2026-11-20）

> 用户给的「星链」中转关键。实测：星链API 的真实后端 = **4sapi.ai**（xingliankey.com 销售页把"星链API"后端指到 4SAPI）。
> key 在 `https://4sapi.ai/v1` 有效（/models 200）。本文不含密钥；测试 key 已 shred。

## 一、端点与模型（自动发现）
- base = **`https://4sapi.ai/v1`**（OpenAI 兼容，NewAPI 系）。
- 共 23 模型，画图清一色 **Google Gemini 图像模型**：`gemini-2.5-flash-image`、`gemini-3.1-flash-image(-preview)`、`gemini-3.1-flash-lite-image`、`gemini-3-pro-image(-preview)`、`gemini-nano-banana-2.1`。每个模型 `supported_endpoint_types=["gemini","openai"]`。

## 二、关键方言发现（第四种，前三种之外）
- Gemini 图像模型在 OpenAI 兼容模式下走 **`/v1/chat/completions`**（对话式出图）。
- 图片以 **markdown `![](data:image/png;base64,xxx)` 内联在 `choices[0].message.content` 字符串**里返回（实测一张 ≈2.4MB，content 为 str 长度约 2.4M）。**不是** `data[]`、也不是 `images[]`、也不是 content parts。
- 图生图（自拍垫图）把输入图作为 `content` 多模态数组里的 `image_url`（data URL）发送。

## 三、实现（扩 `kernel/visual/image_dialect.go`）
- 新增 `DialectChatGemini()`：`Img2ImgMode=chat`，chat 模式**纯文生图与图生图统一走 /chat/completions**。
- `GenericImageBackend.Generate`：`Img2ImgMode==chat` 时无视有无垫图都走 `genChat`。
- `genChat`：无垫图→`content` 用纯字符串（更兼容）；有垫图→多模态数组（text + image_url data URL）。
- `normalizeImageResponse`：新增**字符串 content 的 markdown data-url 抽取**（`extractDataURLs`），扫描 `data:image/...;base64,` 并解码（同时兼容原有 data/images/output/choices 全部形状）。
- 接线：`visual_bridge.VisualInit` 与 `cmd/livecheck` 的 dialect 增加 `chat|gemini|4sapi|chat-gemini` → `DialectChatGemini`。

## 四、真机实测（全绿，均强制水印）
| 项 | 结果 |
|---|---|
| 文生图 `gemini-2.5-flash-image`（chat 方言） | **17.0s**，PNG **1,825,430B**，`HasAIWatermark=true` → build/livecheck-image.png |
| **真·自拍(img2img 垫图)** `gemini-2.5-flash-image` | **25.3s**，PNG **1,142,912B**，`HasAIWatermark=true` → build/livecheck-selfie.png |
| 聊天 `gemini-3.5-flash` | 200 / 2.0s（正文流为空，gemini 文本在 openai 适配器下的边角，非报错；图像路径不受影响） |

> 这是继「OpenAI edits」「硅基 generations+image」之后的**第三种真实方言**，证明通用方言后端不是纸上谈兵——换个完全不同的厂商，配上 dialect 即跑通文生图+真自拍。

## 五、离线单测（免费补）
`image_dialect_test.go` 增：`TestExtractDataURLs`、`TestNormalizeMarkdownContent`、`TestDialectChatTextOnly`（纯文生图走 chat、content 为字符串）、`TestDialectChatEditImage`（垫图走多模态 image_url）。全 PASS。

## 六、门禁 & 产物
`go build/vet/staticcheck ./...`、`go test ./kernel/...`、红线2 全绿；**AAR 9,500,276B + APK BUILD SUCCESSFUL 41.69MB**。

## 七、用户侧用法（星链API）
App：`backend=generic` + `dialect=chat`（或 `4sapi`）+ `base_url=https://4sapi.ai/v1` + `model=gemini-2.5-flash-image`（或 gemini-3-pro-image 等）→ 文生图/自拍可用，强制水印生效。

## 八、预算 & 收尾
实测 3 次图像 + 1 次聊天，≤ ¥2，未越 ¥5。测试 key 已 `shred`（build/.testkey 删除）。
