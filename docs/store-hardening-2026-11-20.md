# 存储层加固：AppendMessage 原子化 + migrate 防降级/断档（2026-11-20）

> 承接 astra 半程评审的 Go 侧 P1（发布前必修，不需真机）。纯本地 + 离线单测，零花费。

## 一、修了什么
### 1) AppendMessage 原子 + updated_at 防倒调（astra P1）
- 旧：`INSERT messages` 与 `UPDATE sessions.updated_at` 是**两条独立写**（两次经写者队列），非原子；
  且 `updated_at=created` 可能**倒调**（乱序/补录时把会话活跃时间拉回过去）。
- 新：经 `Store.execTx` 在**唯一写 goroutine 内以单个事务**执行两条语句一起提交；
  `updated_at=MAX(updated_at, ?)` 取旧新较大值，杜绝倒调。
- 实现：`store.go` 扩 `writeOp.batch`（写者内 `BeginTx`→逐条 Exec→Commit），新增 `execTx`，单条写路径不变（零行为变化）。

### 2) migrate 拒绝「未来 schema / 断档」（astra P1）
- 旧：若数据库 `schema_migrations` 已有更高版本（来自更新版 App）或出现缺口，`migrate` 只是跳过已知迁移，
  会**静默跑在未知/不一致 schema** 上（降级破坏数据风险）。
- 新：读已应用集合后：
  - `maxApplied > LatestVersion()` → 报错「database schema vX is newer than this build supports」拒绝启动；
  - `1..maxApplied` 中任一版本未应用（缺口）→ 报错「schema gap」拒绝启动。

## 二、离线单测（全 PASS）
`kernel/store/store_guards_test.go`：
- `TestAppendMessageUpdatedAtNoBackwards`：updated_at 预置 10000，追加 created=1000 的消息 → 仍 10000（不倒调）；
  再追加 created=20000 → 抬到 20000；消息数正确。
- `TestMigrateRejectsFutureSchema`：注入 version=99 → 重开报错含 `newer`。
- `TestMigrateRejectsGap`：删除 version=2 制造缺口 → 重开报错含 `gap`。

## 三、已确认无需改（astra 点名的其它项）
- `router.Plan()`：已是 `PlanEntry`（明确「无密钥」），红线3 已满足。
- `repo` 多处 `rows.Err()`：已查（93/140/200/240 行等）。
- `characters.go`/`migrate`/`stats` 的 `rows.Err()`：已查。

## 四、门禁 & 产物
`go build/vet/staticcheck ./...`、`go test ./kernel/...`（含 store 新测）、红线2 全绿；
**AAR 重建 + APK BUILD SUCCESSFUL 41.70MB**。

## 五、仍挂 REVIEW-NEEDED（发布前外部人工评审，非本轮）
`store/crypto.go`（AAD 需版本化绑域）、`secret/vault_windows.go`（DPAPI/ACL）、`svc-dist`（manifest 单调序号）、
`native/vector` C 侧、`desktopbind/jnibridge*.c`。这些涉及加密/cgo/签名，按红线7 须外部评审，不自行闭环。
