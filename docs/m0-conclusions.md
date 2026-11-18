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

## T-M0-05 c-shared DLL + JNI 链路（2026-11-18）

**结论：链路通。** Linux .so + JNI 无头 JVM 实测全绿；Windows DLL mingw 交叉编译产物验证齐全，Windows 真机 DEFERRED。

### 实测记录
- 新增 kernel/desktopbind：Go 导出 SrVersion/SrEcho（复用 mobilebind 逻辑，桥层零逻辑）+ 手写 jnibridge.c（REVIEW-NEEDED，红线 7）+ wininc/jni_md.h（交叉编译用最小 Windows JNI 类型头）。
- Linux：go build -buildmode=c-shared -o build/desktop/libstarriver.so ./kernel/desktopbind，exit=0；.so 2.0M + 头文件；nm -D 导出 SrVersion/SrEcho/Java_com_starriver_desktop_KernelBridge_{version,echo} 齐全。
- JNI 实测（javac 17 + java -Xmx256m 无头）：version/echo-ascii/echo-empty/echo-utf8（中文emoji）/echo-control 五用例 [OK]，JNI-LINUX-GREEN。
- Windows：CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -buildmode=c-shared -o build/desktop/starriver.dll，exit=0；file 验货 PE32+ executable for MS Windows 10.00 (DLL), x86-64, 21 sections；mingw-nm 四符号齐全。
- 一次通过，无报错。

### 验收映射（DEFERRED 延伸，依 T-M0-01 裁决）
- PRD"Compose Multiplatform 桌面工程经 JNI 调 Echo"：JNI ABI 已经 Java 载体无头验证（与 Kotlin 同一 JNI 符号路径）；Compose 桌面工程 M5/T-M5-03 落地，G0 前以 Xvfb 无头补验最小 Compose+JNI 工程（4G 内存约束，Compose 桌面全量工程不在 M0 跑）。Windows 真机调 DLL 仍 DEFERRED。

### 遗留 / 待裁决
- jnibridge.c 为手写 cgo/JNI 指针边界：REVIEW-NEEDED，外部评审前不得视为终稿。
- JNI NewStringUTF 用 Modified UTF-8：emoji 往返实测无损，但与非 JVM 方互操作需改 String.getBytes(UTF_8)+byte[] 方案，M5 裁决。
- JDK 头路径硬编码构建机路径（main.go #cgo 注释已标），M5 参数化。
- wininc/jni_md.h 为最小替代头，M5 换正式 Windows JDK 头复核。
- DLL 在 Windows 真机的加载/调用 DEFERRED；Go runtime 在 Windows 服务/托盘场景行为 M5 验。

### 复现步骤
1. go build -buildmode=c-shared -o build/desktop/libstarriver.so ./kernel/desktopbind
2. cd ui-desktop && javac -encoding UTF-8 -d build/classes src/main/java/com/starriver/desktop/*.java
3. java -Xmx256m -cp build/classes com.starriver.desktop.Main /root/star-river/build/desktop/libstarriver.so → JNI-LINUX-GREEN
4. CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -buildmode=c-shared -o build/desktop/starriver.dll ./kernel/desktopbind

## T-M0-09：garble 兼容性实测（spike，已批准提前）

### 结论（garble v0.18.0 + Go 1.27.4，构建机实测）
- **Linux 桌面 .so：兼容。** `garble build -buildmode=c-shared` exit=0，体积 2.0M→1.4M；`nm -D` 动态符号 SrVersion/SrEcho/Java_com_starriver_desktop_KernelBridge_version/echo 全在；JNI 五用例复跑全绿（JNI-LINUX-GREEN）。
- **Windows DLL：兼容（导出表级验证）。** mingw 交叉 + garble exit=0；garble 剥符号表（21→10 sections，file 报 stripped），`nm` 无输出属预期；`objdump -p` PE 导出表四名齐全（SrVersion/SrEcho/两个 Java_com_...）。Windows 真机加载仍 DEFERRED，M5/T-M7 复核。
- **Android gomobile：不兼容，无注入点。** `go tool gomobile help bind` 共享 build flags 仅 -a/-n/-x/-gcflags/-ldflags/-overlay/-tags/-trimpath/-work，**无 -toolexec**（garble 的注入机制）；`garble bind` → `unknown command: "bind"`。
- **降级方案（采纳并验证）：仅桌面 garble。** Linux .so 与 Windows DLL 走 garble，Android AAR 链路不混淆。T-M7 桌面发布构建用 garble 替换 go build，Android 链路不变。

### 遗留 / 风险
- Android 端 Go 符号不混淆：闭源包（hippocampus/endocrine/proactive）进 AAR 后 libgojni.so 内包路径/函数名仍可读。缓解候选（fork gomobile 加 toolexec / 等上游支持）未批准，v1 不引入；记残余风险，T-M7-06 复查。
- garble 剥 DLL 调试符号影响 Windows 崩溃解析（T-M7-08）：minidump 符号化方案需在 M7 用 garble 产物复核。
- 本 spike 仅覆盖 desktopbind（Version/Echo）。闭源包接入后需重跑 garble 全量构建验证（T-M7）。

### 复现步骤
1. `garble build -buildmode=c-shared -o build/garble/libstarriver.so ./kernel/desktopbind`
2. `nm -D --defined-only build/garble/libstarriver.so | grep -E 'Java_|SrVersion|SrEcho'` → 四导出符号在
3. `javac -encoding UTF-8 -d build/classes ui-desktop/src/main/java/com/starriver/desktop/*.java && java -Xmx256m -cp build/classes com.starriver.desktop.Main /root/star-river/build/garble/libstarriver.so` → JNI-LINUX-GREEN
4. `CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc garble build -buildmode=c-shared -o build/garble/starriver.dll ./kernel/desktopbind`
5. `x86_64-w64-mingw32-objdump -p build/garble/starriver.dll | grep -A 15 'Export Address Table'` → 导出表四名齐全
6. `go tool gomobile help bind`（flags 无 -toolexec）；`garble bind ...` → unknown command: "bind"

## T-M0-07：cgo 集成 device_probe 双端跑通

### 实测
- kernel/native/bridge：cgo 按 GOOS/GOARCH 链 build/native/<target>/libdevice_probe.a；Linux `go test -race` 两用例 PASS（neon=false avx2=true）。
- Linux .so 2.0M→2.4M，nm 含三个 T sr_ + SrDeviceProfile + Java_...deviceProfile；JNI 六用例全绿，DEVICE-PROFILE JSON 打印一次（avail≈617M）。
- Windows DLL 2.7M→3.4M，mingw-nm 同五符号齐全；真机打印 DEFERRED（裁决映射：符号级验证代替，M5/T-M7 补真机）。
- AAR：libgojni.so 2,614,552→2,622,296 字节，llvm-nm 含三个 T sr_ + cgo 包装符号；APK 12M（较 M0-04 的 9.8M 增大，T-M7-06 体积复查）。
- 主人真机（arm64）覆盖安装实测：PROFILE {"neon":true,"avx2":false,"avail_mem_bytes":162447360}，与构建机 x86 结果相反，架构检测交叉验证成立。截图在会话。

### 指针规则评审记录（红线 7，REVIEW-NEEDED）
- 范围：kernel/native/bridge/bridge.go（cgo 调用边界）、kernel/desktopbind/main.go 新增 SrDeviceProfile、kernel/desktopbind/jnibridge_probe.c（新增 JNI 桥）。jnibridge.c 维持 T-M0-05 挂起评审，本任务未改动。
- C 不持有 Go 指针：bridge 三函数全为传值调用（void 入参、标量返回），无指针入参、无句柄、无堆分配、无回调；C 侧无从获得 Go 指针。
- SetFinalizer 兜底：不适用——device_probe 无句柄无资源，没有可终结对象；不造假句柄凑验收。约定：未来 C API 引入句柄/资源型返回时，Go 封装必须持句柄并 SetFinalizer 兜底 + 显式 Close，届时重走评审。
- SrDeviceProfile 内存契约同 SrVersion：C.CString(malloc) 返回，jnibridge_probe.c 侧 NewStringUTF 后 free；NULL 防御；无入参故无 GetStringUTFChars 路径；JSON 全 ASCII 无 Modified UTF-8 风险。
- 结论：自评审通过，待外部评审确认。评审前 jnibridge_probe.c 与 bridge.go 的 cgo 段不视为终稿。

### 遗留
- APK 体积 9.8M→12M 待 T-M7-06 复查（libgojni 未 strip + cgo 桥增量）。
- bridge 的 LDFLAGS 依赖先跑 make -f native/Makefile all；CI 脚本顺序 T-M0-08/10 固化。
