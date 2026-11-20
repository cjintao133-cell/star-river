# 跨厂商图像方言自适应（第一步）（2026-11-20）

> 解决「换中转站/官方模型后自拍/生图会崩」的根因：图像 API 跨厂商不是同一方言。
> 本步纯新增（不动已验证的 OpenAIImageBackend / SiliconFlowImageBackend 快路径），零回归。

## 一、根因（四类必炸点）
1. **img2img 端点碎片化**：官方 `/images/edits`(multipart) vs 硅基/千问 `/images/generations`+`image`字段(data URL) vs chat 型出图；很多中转站无 edits→404。
2. **尺寸不认**：DALL·E 3 / gpt-image-1 只认固定档；自拍模板写死 768×1024 → 打官方 400。
3. **参数名/开关不认**：size / image_size / width,height；response_format 有时不认。
4. **返回结构五花八门**：data[].b64_json / data[].url / images[].url / output[].result / choices[].message。

## 二、本步实现（`kernel/visual/image_dialect.go`）
- **`ImageDialect`**：把差异收敛为配置（Img2ImgMode = edits|generations|chat；SizeFields=size|image_size|wh；SendResponseFormat）。
- **Presets**：`DialectOpenAI()`（edits+size+response_format）、`DialectSiliconFlow()`（generations+image，image_size+size 双写，不发 response_format）、`DialectGeneric()`（保守默认）。
- **`GenericImageBackend`**：一个后端按 Dialect 分派生成（文生图 + 三种 img2img），复用同包 toDataURL/fetchURLBytes/netx。
- **`normalizeImageResponse`**：一套解析器吃下上述全部返回形状（含 data 为 url 字符串、chat content parts）。
- **`snapImageSize`**：把 (w,h) 吸附到 DALL·E 3 / gpt-image-1 / DALL·E 2 支持的尺寸档；未知模型原样透传。

## 三、接线
- `kernel/mobilebind/visual_bridge.go` `VisualInit`：新增 `backend: generic`（别名 generic-image/openai-compatible/compatible）+ `dialect` 字段（openai|siliconflow|generic），可选 `edit_path` 覆盖。App 侧一家中转站只需配 backend+dialect，不再写后端类。
- `cmd/livecheck`：新增 `-backend generic -dialect ...`，`image/edit/selfie` 均可走 generic 验证。

## 四、测试（离线 httptest，免费，全覆盖）
`kernel/visual/image_dialect_test.go`：
- `TestNormalizeShapes`：data_b64 / data_url / images_url / output_res / data_strurl / chat_imgs / chat_parts 七种返回全解析为图。
- `TestDialectGenerationsImage`：断言 body 带 `image` data URL、`image_size`+`size`、**不发** response_format（硅基方言）。
- `TestDialectEditsMultipart`：断言 multipart 含 image 文件 + prompt + model + response_format（官方方言）。
- `TestDialectChat`：断言 chat body 含 image_url。
- `TestSnapImageSize`：dall-e-3 / gpt-image-1 吸附到固定档，未知模型透传。
- `TestGenericHTTPError` / `TestNormalizeError`：错误路径。
- **顺带修复真 bug**：`GenericImageBackend.setHeaders` 原会把 multipart 的 Content-Type 覆盖成 application/json（导致 edits 失败），单测逮到并修正（setHeaders 不再设 Content-Type，doJSON 单独设 json）。

## 五、门禁
`go build/vet/staticcheck ./...` 全绿；`go test ./kernel/...` 全绿（visual/mobilebind 含新测）；红线2 PASS；**AAR 9,505,355B + APK BUILD SUCCESSFUL 41.68MB**。

## 六、边界与下一步
- 本步为离线验证；已知引擎（OpenAI 严格 / 硅基）真机已在 docs/live-selftest 验过，generic 走 DialectSiliconFlow 与之等价（单测复刻其请求/返回）。
- **第二步（待定，需时再做）**：`VisualInit` 能力探测（探测端点支持 edits/image/generations）+ 自拍前「不支持垫图→置灰或降级纯文生图」的优雅降级，避免运行时炸给用户。
- 未做：把 OpenAIImageBackend / SiliconFlowImageBackend 折叠进 GenericImageBackend（可作后续精简，非必需）。
