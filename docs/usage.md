# Star River（星河）· 使用说明

本文档只描述**当前主树（内核/框架）真实可用的东西**。UI 已移出主树（见 FAQ）。

## 0. 环境前置

- **Go**：`go.mod` 要求 **go 1.26.0**（务必用 ≥1.26 的工具链；旧版本会构建失败）。
- **C 工具链**：`cc`/`gcc`（构建 `native/` 热点模块；不带 native 时可只跑纯 Go 部分）。
- 可选：`staticcheck`（静态检查，需自行 `go install golang.org/x/tools/cmd/staticcheck`）。
- Android 构建额外需要：NDK r26+、JDK17、Gradle 8.9（仅在你要出 AAR 时）。

## 1. 获取与构建

```sh
git clone <repo> star-river && cd star-river

go build ./...          # 编译全仓
go vet ./...            # 静态检查
go test ./kernel/...    # 内核单测（并发包建议加 -race）

# 分层依赖守卫（底座不依赖伴侣插件包）
sh scripts/check-open-split.sh

# C 热点模块（可选，生成三平台静态库到 build/native/）
make -f native/Makefile all vector
```

## 2. 跑通最小对话（srchat）

`cmd/srchat` 是最小 CLI 聊天闭环（七层上下文最小版 + openai 兼容流式）。**key 只从文件或环境变量读，绝不进命令行/日志。**

```sh
# 方式 A：云端端点（BYOK，例如任意 OpenAI 兼容中转）
echo -n "sk-你的key" > /root/myk
go run ./cmd/srchat \
  -base https://your-host/v1 \
  -model your-model-id \
  -keyfile /root/myk

# 方式 B：环境变量给 key
SR_API_KEY=sk-你的key go run ./cmd/srchat -base https://your-host/v1 -model your-model-id
```

参数一览：

| 参数 | 说明 |
|---|---|
| `-base` | **必填**。OpenAI 兼容端点 base，形如 `https://host/v1` |
| `-model` | **必填**。模型 id |
| `-keyfile` | API key 文件路径（或用环境变量 `SR_API_KEY`） |
| `-persona` | 可选。人设系统提示 |
| `-once "你好"` | 单发一条后退出；留空则进入 REPL（`:quit` 退出） |
| `-maxtokens` | 单次生成上限，默认 1024 |
| `-maxmsgs` / `-maxchars` | 上下文消息条数 / 字符预算，默认 20 / 8000 |
| `-timeout` | 单次请求超时，默认 90s |

## 3. 完全离线跑（接本地模型）—— 这才是"真·纯本地"

Ollama / LM Studio 都提供 **OpenAI 兼容接口**，把 `-base` 指过去即可全程不联网：

```sh
# 以 Ollama 为例（默认监听 127.0.0.1:11434，OpenAI 兼容前缀 /v1）
ollama pull qwen2.5:7b

go run ./cmd/srchat \
  -base http://127.0.0.1:11434/v1 \
  -model qwen2.5:7b \
  -keyfile /dev/null        # 本地模型通常不校验 key，随便给个空文件即可
```

> 此时 prompt 只在本机内网/环回地址流动，**不出设备**。这就是本地内核 + 本地推理的"全离线"形态。

## 4. 真机/真实端点冒烟（livecheck）

```sh
# 纯本地、免费：验证水印管线
go run ./cmd/livecheck -mode selftest

# 真实端点：1 次对话（证明 endpoint/key/协议通）
go run ./cmd/livecheck -mode chat  -run -base https://host/v1 -model your-model -keyfile /root/myk

# 真实端点：1 次生图 → 强制水印 → 断言 HasAIWatermark
go run ./cmd/livecheck -mode image -run -base https://host/v1 -model your-model -imgmodel your-image-model -keyfile /root/myk
```

## 5. 导入角色卡

`kernel/cards` 支持四载体：

- **V2 / V3 JSON**：`spec`/`spec_v2` + `data` 嵌套。
- **chara PNG**：PNG `tEXt`/`iTXt` chunk 内嵌 base64 卡 JSON（`ccv3` 优先 `chara`）。
- **charx**：zip 内 `card.json`（或任意 json 兜底）。
- **世界书**：`Constant` 常驻 + 关键词激活（递归深度上限 + 循环检测）。

（具体 API 见 `kernel/cards` 包的 `Parse` / 结构体。）

## 6. 嵌入到你自己 App（进阶）

内核可编译成移动/桌面库，由你写 UI：

- Android：`gomobile bind` 内核（`kernel/mobilebind` 暴露的 API）→ AAR。
- Windows/桌面：`go build -buildmode=c-shared` → DLL + JNI 桥（`kernel/desktopbind`）。

## 7. 密钥与隐私（红线）

- 密钥只经 `-keyfile` 或环境变量 `SR_API_KEY` 输入；**不进 argv、不进日志**（代码内还有输出脱敏双保险）。
- 适配器统一**禁止跟随重定向**（防凭据跨域外泄）。
- 数据去向 = 你填的 `-base`；详见 `docs/privacy-boundary.md` 与 `docs/product-brief.md`。

## 8. 常见故障

| 现象 | 原因 / 处理 |
|---|---|
| 构建报 go 版本错 | 工具链低于 1.26；升级 Go |
| 启动报"未提供 key" | 没给 `-keyfile` 也没设 `SR_API_KEY` |
| 启动报"必须提供 -base 与 -model" | 必填参数缺失 |
| 连本地模型报错 | `-base` 需带 `/v1`（OpenAI 兼容前缀）；确认 ollama 已启动 |
| 某些 Gemini 文本模型返回 200 但正文空 | 已知边角，见 `KNOWN-ISSUES.md` |
