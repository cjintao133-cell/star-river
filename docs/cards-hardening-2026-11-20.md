# 角色卡导入加固：ZIP 炸弹防护 + PNG 边界（2026-11-20）

> astra P1：charx(zip) 未限条目数/累计解压量（zip 炸弹 OOM）；PNG chunk 边界用 int（32 位不牢）。纯本地离线单测。

## 一、charx（zip）炸弹防护（kernel/cards/charx.go）
旧：仅单文件 `LimitReader(32MB)`；未限条目总数、未限跨文件累计解压量 → 成千上万个小 json 各读 32MB 可放大 CPU/IO/内存。
新：加双重预算——
- `maxZipEntries=4096`：`len(zr.File) > 4096` 直接 `ErrBadFormat`（zip 炸弹特征）。
- `maxJSONScan=512`：最多扫描 512 个 json 文件即停。
- `maxPerJSONBytes=32MB` / `maxTotalJSONByte=64MB`：单文件与**累计**解压双重上限，累计耗尽即停。
（正常卡远小于此，无行为影响。）

## 二、PNG chunk 边界（kernel/cards/png.go）
旧：`length := int(uint32)`、`dataEnd := dataStart + length` 用 int 计算，32 位平台可能回绕，越界判断不可靠。
新：整段改用 **uint64** 计算 `length/dataEnd/n`，越界判断 `dataEnd+4 > n` 无符号、跨架构安全；`off` 末尾转回 int。

## 三、离线单测（全 PASS）
`kernel/cards/cards_guard_test.go`：
- `TestParseCharxValid`：正常 zip（card.json + asset）仍解析出卡（回归）。
- `TestParseCharxTooManyEntries`：4097 条目 → `ErrBadFormat`。
- `TestParsePNGHugeChunkNoPanic`：chunk 声明 0xFFFFFFFF、数据不足 → 不 panic、优雅返回错误。

## 四、门禁 & 产物
`go build/vet/staticcheck ./...`、`go test ./kernel/...`（含 cards 新测）、红线2 全绿；**AAR + APK BUILD SUCCESSFUL 41.70MB**。

## 五、仍挂（后续/外部评审）
- `store/crypto.go` AAD 版本化绑域、`secret/vault_windows` DPAPI/ACL、`svc-dist` manifest 单调序号、`native/vector` C 侧、`jnibridge*.c` —— 红线7 外部人工评审。
- orchestrator 记忆出站分级（红线5，设计较重）、adapter 多 choice/流式总量上限 —— 需单独推进。
