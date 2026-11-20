# astra 终扫 + C 边界加固记录（2026-11-20）

> 承接 `docs/astra-review-2026-11-20.md`（半程 30/60 全仓评审）。本轮为「终扫」：
> 只针对半程评审之后**真正没被审过的 C 层**做二次外审并整改。降本增效口径：
> 先本地免费门禁跑穿 → 只把未覆盖的 4 个文件喂 astra，不重复烧已审内容。
> 基线 HEAD `11c1bd8` → 整改 `c3ee579`。

## 一、为什么这么扫（降本增效）
- 半程 `astra-review-2026-11-20.md` 已覆盖全仓 64 个生产文件、给出 P0/P1 清单；
  上一轮 `11c1bd8` 按 P1 修了 7 条（含 `vector.go` Go 层、`jnibridge_kernel.c`）。
- **但 `native/vector/vector.c`（C 实现本身）从不在任何 delta 里**——`11c1bd8` 的
  stat 未触及它。astra 半程把 `native/vector/vector.go + vector.c` 判为「不放行」，
  其 C 侧 P0（双释放/容量溢出/点积溢出）**始终没被专扫过**。这才是终扫真正该打的地方。
- 所以本轮 scope = `native/vector/vector.c` + `vector.h` + `kernel/native/vector/vector.go`
  + `kernel/desktopbind/jnibridge_kernel.c`。其它已审 Go 层（crypto / vault_windows /
  keybackup / svc-dist）不复读——它们本就挂「发布前外部人工评审」红线7，等改动时再单扫。

## 二、免费门禁（0 成本，先跑）
- `go build ./...` ✓　`go vet ./...` ✓　`staticcheck ./...` 干净 ✓
- `scripts/check-open-split.sh`（红线2）**PASS**：开源区单独可编译 + 零闭源依赖 ✓

## 三、astra 终扫（真账单）
- 命令：`astra-scan -files "native/vector/vector.c,native/vector/vector.h,kernel/native/vector/vector.go,kernel/desktopbind/jnibridge_kernel.c" -q "只做终扫…"`
- `-dry` 预估：prompt 33,683 字符 ≈ 11,227 token（上限 20,000，未触闸）
- **实跑 usage：prompt 10,993 / completion 1,900（含 reasoning 537）/ total 12,893**
- 折价约 **¥5 上下**（以中转站账单为准）。对比全量 16 文件 ≈¥14 / 全仓 ≈¥93，本轮回合只花零头。

## 四、结论分档（工程师复核后，非照搬）
**✅ 真·双释放：已消除。** `Index` 只持一个 `*vecOwner`，锁/句柄/closed/finalizer 全在
owner 里，按值复制只复制指针 → 单一 owner，无双句柄双释放。复核 `vector.go` 确认属实。

**🔴 真 P1（已修）——`jnibridge_kernel.c` `nInitKernel` pending 异常短路漏一处：**
原代码 `db` 转完 `cdb` 后，不管 `sd==JB_EXC` 都继续去转 `key`；等两次都转完才
`ExceptionCheck`。即 db 已挂 pending 异常，仍带异常调 `GetArrayLength`（JNI UB），
且与文件头注释「有 pending 异常绝不再调 JNI」自相矛盾。同文件 `nCreateSession/nChatOnce`
等方法均已逐段短路，**唯独 nInitKernel 漏**。→ 已对齐修复。

**🟡 P1 理论/低危（已兜底）——`vector.c` 点积未宽累加：**
`dot_int8_scalar/neon` 累加器与调用点 `d` 均 `int32`，全范围 int8 乘积 16384，
安全维度上限 2^31/16384 = **131071**；超过才有符号溢出。嵌入维度实际仅数百~1.5k，线上不触发。
→ 采最小侵入兜底：C 与 Go 入口统一 `dim ≤ 131071`，不改点积逻辑（若未来要超大维度再整改 int64）。

**🟠 条件性 P0（仅 32 位 size_t，已消）——容量/帧长 size_t 回绕：**
`ensure_cap` 的 `ncap*dim`、序列化 `12+count*per` 在 32 位下可能回绕后按短长度放行。
我们只出 arm64/amd64 → 已在 `vector.c` 顶部加**编译期 `#error` 拒绝 32 位 size_t**，把
「靠构建惯例排除 32 位」这个隐患直接钉死。

**⚪ 记录不改：** `Deserialize` 原 `C.CBytes` 前不验帧长（已顺手加 12 字节头+magic+条数/帧长预校验）；
`Search` 未 `min(k,count)`（k 已封 1024，最多浪费 ≤12 KiB）；`JB_OOM/JB_NUL` 对外折叠为
`ERR:jni-badarg`（可观测性小瑕疵）。

**底线：无新真 P0 阻断。** astra 标的「P0」仅在 32 位成立，现被编译期断言堵死。

## 五、本轮整改（commit `c3ee579`，3 文件 +39/−8）
1. `kernel/desktopbind/jnibridge_kernel.c`：`nInitKernel` 逐段短路——`db` 转换后若 `sd==JB_EXC`
   立即 `finish_err(has_exc=1)` 仅做 C 清理返回 NULL，不再带 pending 异常转 `key`/调 JNI。
2. `native/vector/vector.c`：顶部 `#if !defined(SIZE_MAX) || SIZE_MAX < 0xFFFFFFFFFFFFFFFFULL`
   → `#error`（仅 64 位）；`#define SR_VEC_MAX_DIM 131071`；`create` 与 `deserialize` 均加 `dim > SR_VEC_MAX_DIM` 拒绝。
3. `kernel/native/vector/vector.go`：`const maxDim = 131071`；`NewIndex` 用 `maxDim`；
   `Deserialize` 先校验 `len≥12` + `magic=="SRV1"` + `dim∈(0,maxDim]` + `count≥0` + 帧长
   `count ≤ (len-12)/(16+dim)`，通过后才 `C.CBytes`；新增 `countFromSerialized`。

## 六、验证证据（全绿，可复现）
- **C 单测**：`make -f native/Makefile vector-test && ./build/vector-test` → `VEC-TESTS-GREEN`
- **ASan**：`gcc -fsanitize=address` 构建运行 → exit 0，无泄漏/越界
- **Valgrind**：`valgrind --leak-check=full --error-exitcode=99` → exit 0
- **C 编译**：`-O2 -Wall -Wextra -Werror -std=c11`（linux-amd64 + android-arm64）零告警
- **Go**：`go build ./...` ✓　`go vet ./...` ✓　`staticcheck ./...` 干净
- **Go 测试**：`go test ./kernel/...` 零失败（`kernel/native/vector` ok）
- **桌面 JNI 门禁**：`scripts/check-desktop-jni.sh` → `DESKTOP-JNI-GATE: PASS`（init/角色/会话/emoji 无损全 OK）
- **AAR**：`gomobile bind -target=android/arm64 -androidapi 21` → `build/mobilebind.aar` **9,467,845 B**
- **APK**：`./gradlew assembleDebug` → `BUILD SUCCESSFUL`，`app-debug.apk` **41,613,103 B**

## 七、仍属红线7 外部评审（本轮不关闭）
本次改的两个文件本身仍是 REVIEW-NEEDED，需发布前外部人工评审，**本轮只降风险、不替代评审**：
- `native/vector/vector.c`（cgo/C 侧；Go 层 P0 已修，C 侧加维度上限与 64 位断言后风险显著下降）
- `kernel/desktopbind/jnibridge_kernel.c`（手写 JNI 指针边界）

## 八、下一步（按序，发话再动）
1. 其余 REVIEW-NEEDED：`crypto` / `vault_windows` / `keybackup` / `svc-dist` 的外部评审（红线7，发布前必修）。
2. 若要「超大维度向量」再整改 `int64` 点积（当前 131071 上限对伴侣维度绰绰有余）。
3. 归档定稿后回归 M7 剩余真机/物料清单（用户设备 handling）。
