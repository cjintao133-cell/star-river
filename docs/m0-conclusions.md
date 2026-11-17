# M0 结论记录

> 每任务追加一节；只记实测，未跑不记。

## T-M0-04 gomobile AAR 链路（2026-11-18，真机验证通过）

**结论：链路全通。** gomobile bind → AAR → Gradle/AGP → APK → 主人真机调用成功。

### 实测记录
- 首次 bind 报错 `gomobile: missing golang.org/x/mobile dependency`；新版 Go 要求 go.mod 登记 tool。修复：`go get -tool golang.org/x/mobile/cmd/gomobile && go mod tidy`。
- go.mod `go` 指令被 x/mobile 抬至 **1.26.0**（构建机 Go 1.27.4 满足；PRD 下限 1.22+ 不违反）。随带间接依赖 x/mod v0.43.0、x/sync v0.25.0、x/tools v0.53.0（工具链内部用，非新增业务依赖）。
- bind 命令：`gomobile bind -target=android/arm64 -androidapi 21 -o build/mobilebind.aar ./kernel/mobilebind`，exit=0。
- AAR 1.5MB；unzip -l 验货：jni/arm64-v8a/libgojni.so 2,614,552 字节；APK 内同路径同字节数，一致。
- 安卓工程：Gradle 8.9（腾讯内网镜像）+ AGP 8.5.2 + Kotlin 2.0.21 + Compose BOM 2024.09.03；依赖仓库阿里云镜像优先、google/mavenCentral 兜底；wrapper distributionUrl 锁腾讯镜像。
- 内存纪律实测：jvmargs -Xmx1g、daemon=false、parallel=false、workers.max=2、kotlin in-process；assembleDebug 1m30s / 36 tasks，无 OOM，APK 9.8MB。
- 警告一条：Unable to strip libgojni.so（NDK strip 未处理，原样打包，无功能影响；T-M7-06 复查体积）。
- 真机（主人手机，arm64，SFTP 侧载 debug APK）：屏幕显示 VERSION: star-river-kernel/0.0.1-m0 与 ECHO: ping-中文-🌌，与 Go 侧单测断言一致，字符串双向无损。截图存会话记录。
- Go 门禁：go test -p 2 / go vet / staticcheck 全绿（M0-04-UNIT-GREEN）。

### 桥接纪律（后续任务遵守）
- kernel/mobilebind 只做薄门面，函数签名稳定；内核逻辑不得写在桥层。
- gomobile 导出类型受限（string/数值/bool/[]byte 等）；复杂结构过桥方案（JSON 字符串 vs 导出对象）M1 接真内核时裁决。
- 当前仅 arm64-v8a；其余 ABI 是否进发布矩阵 T-M7-06 定。

### 复现步骤
1. go test -p 2 ./kernel/mobilebind/
2. gomobile bind -target=android/arm64 -androidapi 21 -o build/mobilebind.aar ./kernel/mobilebind
3. cp build/mobilebind.aar ui-android/app/libs/
4. cd ui-android && ./gradlew assembleDebug --no-daemon
产物：app/build/outputs/apk/debug/app-debug.apk（AAR/APK 为构建工件，不入 git）。
