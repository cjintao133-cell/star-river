# 密钥安全存储设计（T-M1-09）

> 红线 3（日志/错误/提交不含 key）、红线 4（key 只经 Keystore/DPAPI 封装读写、不落 SQLite 明文）、红线 7（加密实现 diff 标 REVIEW-NEEDED）的落地。

## 一、契约

- 所有 key/敏感字段只经 `kernel/secret.Vault` 接口读写。
- **持久化层（store，未来 SQLite）只存 `secret.SecretRef`（一个 id 字符串），绝不存 secret 本体。**
  字段类型用 `SecretRef`，从类型层面杜绝把明文写进会话/配置表。
- 使用时：从 Vault 取回 secret → 交给适配器（适配器只在内存用，不落盘、不进 URL/日志）→ 用完即弃。
- 落盘/落日志前，任何可能含 secret 的字节用 `secret.Scrub` / `ScrubString` 清洗（双保险）。

## 二、统一接口

```go
type Vault interface {
    Set(id string, secret []byte) error
    Get(id string) ([]byte, error)   // 不存在返回 ErrNotFound
    Delete(id string) error          // 幂等
}
```

## 三、平台后端（同一接口，可插拔）

| 后端 | 文件 | 说明 | 状态 |
|---|---|---|---|
| MemVault | vault.go | 内存实现，非持久；仅测试/本地开发 | 已实现、已测（linux 100%） |
| DelegateVault | vault_delegate.go | 把平台 Delegate 注入为 Vault；统一校验 | 已实现、已测 |
| Windows DPAPI | vault_windows.go (`//go:build windows`) | DPAPI(当前用户范围) 加密 + 密文落目录 | **交叉编译通过；运行期 DEFERRED；REVIEW-NEEDED（加密实现，红线7）** |
| Android Keystore | （经 Delegate） | Keystore 密钥在安全硬件，加解密由 Kotlin 侧 `android.security.keystore` 完成，经 `kernel/mobilebind` 门面把 Set/Get/Delete 注入 DelegateVault | 接口就绪；Kotlin 侧实现在 UI/桥层，M5 落地，运行期 DEFERRED |

### Windows DPAPI 说明（REVIEW-NEEDED）
- 纯 `syscall` 调 `crypt32.dll` 的 `CryptProtectData`/`CryptUnprotectData`（`LocalFree` 释放），**零新依赖**（红线 1）。
- 只对 `//go:build windows` 可见；Linux 构建机用 `GOOS=windows go build/vet/staticcheck` 验编译与静态检查，**无法运行期验证 → runtime DEFERRED**。
- 属红线 7「加密实现」类 diff：**外部评审前不视为终稿，不得自行合并**。
- 密文目录建议 `%APPDATA%/star-river/secrets`，文件 0600，id 经 `sanitizeID` 白名单化防路径穿越。

### Android Keystore 说明
- Go 内核不直接碰 JNI/Keystore：Keystore 的密钥必须在 Android 进程侧生成使用。做法是 Kotlin 用 `KeyStore.getInstance("AndroidKeyStore")` + AES/GCM 加解密，密文存 app 私有目录，通过 gomobile 门面把一个实现 `secret.Delegate` 的对象方法注入 Go 的 `DelegateVault`。**Go 侧只依赖 `Delegate` 接口，平台代码留平台层。**

## 四、"不落 SQLite 明文"双重断言（persistence_test.go）

1. **单测**：按契约构造要落盘的 `persistedConfig`（只含 `APIKeyRef`），序列化写临时"数据库文件"，断言 secret 只活 Vault、经 ref.id 可取回。
2. **文件字符串扫描**：扫描该"数据库文件"原始字节，断言**不含 secret 明文**；并附**反向对照**（故意把 secret 写进结构，扫描必须抓到）证明扫描非空断言；另测 Scrub 后落日志文件无明文。

> store（SQLite）尚未建（M2/T-M2-01）。本任务把"只存引用、不存明文"的契约与断言先用临时文件模拟落地；待 T-M2-01 建库后，把 `persistedConfig` 换成真实 schema、`dbPath` 换成真实库文件即可复用同一断言。

## 五、遗留 / DEFERRED

- Windows DPAPI 运行期验证（真机加密/解密往返）DEFERRED 至 M5（与其它 Windows 运行时项一致）。
- Android Keystore 的 Kotlin 实现与 gomobile 注入接线 M5 落地。
- 真实 SQLite 库文件的字符串扫描断言待 T-M2-01 建库后接入（契约与测试骨架已备）。
- DPAPI 加密实现：REVIEW-NEEDED（红线 7）。
