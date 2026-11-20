# 图像能力探测 + 优雅降级（第二步）（2026-11-20）

> 第一步把"跨厂商方言"收敛成可配置；第二步把"模型不支持某项能力就 404 炸给用户"
> 变成「提前识别 → 自动降级/明确告知」，运行时绝不再崩。纯本地 + 离线单测，零联网零花费。

## 一、要解决的问题
- 有的模型能出图但不支持垫图（如 DALL·E 3 无 edits），自拍传垫图会 400/404。
- 有的模型根本不是画图模型（tts/embedding），误配会白烧一次请求才报错。
- 用户不知道"为什么失败/为什么效果不像"。

## 二、实现
1. **能力表** `kernel/visual/imcap.go`：`ModelCapability(model) → {CanImage, CanImg2Img}`
   - 非图像（tts/whisper/asr/embed/rerank/bge/ocr/moderation/clip）→ `{false,false}`；
   - 已知不支垫图（dall-e-3）→ `{true,false}`；
   - 其余（含未知，乐观）→ `{true,true}`。
2. **降级路径** `SelfiePipeline.SubmitTextOnly(ctx, sceneKey, appearance)`：
   - 模型不支持垫图时改走纯文生图：不传 ImageRef，提示词追加"人物外貌参考：<角色描述/名字>"，
     `Extra.degraded=true`，其余（场景/模板/强制水印）不变。
3. **自拍自动降级** `visual_bridge.VisualSelfie`：
   - `VisualInit` 记住 model/backend/dialect；
   - 调自拍前 `ModelCapability(model)`：`!CanImage` → 直接友好报错（不再白烧请求）；
   - `CanImg2Img` → 正常垫图；否则 → `SubmitTextOnly` 降级；
   - 结果 JSON 增 `degraded:bool` + `note:string`（说明"已降级、一致性可能下降、建议换模型"）。
4. **能力查询桥** `visual_bridge.VisualCapabilities() → {configured,model,backend,dialect,canImage,canImg2Img}`
   （供 UI 预先判断是否显示/置灰自拍）。
5. **UI** `AlbumScreen.kt`：自拍成功且 `degraded=true` 时状态栏显示 `⚠ + note`，用户可见降级原因。

## 三、离线单测（免费，全 PASS）
`kernel/visual/imcap_test.go`：
- `TestModelCapability`：dall-e-3→{true,false}，dall-e-2/gpt-image-1/gemini-*-image/Kolors/Qwen-Image-Edit→{true,true}，
  whisper/embedding/bge→{false,false}，空→{true,true}。
- `TestSelfieSubmitTextOnly`：降级请求不传 ImageRef、提示词含场景+外貌、`Extra.degraded=true`、产物仍带水印。

## 四、门禁 & 产物
`go build/vet/staticcheck ./...`、`go test ./kernel/...`、红线2 全绿；**APK BUILD SUCCESSFUL 41.70MB**（含新桥 + UI 降级提示）。

## 五、行为矩阵（最终）
| 模型能力 | 自拍行为 |
|---|---|
| 支持垫图（gemini-*-image / gpt-image-1 / Kolors / flux / sd / Qwen-Image-Edit） | 正常垫图，角色一致 |
| 能出图但不支持垫图（dall-e-3 等） | **自动降级纯文生图** + 明确告知 |
| 非画图模型（tts/embed…） | **执行前友好报错**，不烧请求 |
| 未知模型 | 乐观按支持垫图执行；真错走既有 OnError 不崩 |

## 六、边界
- 能力表是**静态启发式**（按模型名）。未知模型乐观放行——真正不支持时仍会有一次请求失败，但被既有错误分类优雅处理，不崩、不掩盖。
- 更精确的"主动探活"（发一次极小请求探测）会花钱且慢，本步**默认不做**；如需可后续做成显式"探测"按钮。
