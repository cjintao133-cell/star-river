/* T-M0-10：Windows 最小 exe 启动器（仅交叉编译产物，真机运行 DEFERRED）。
 * 用法：与 starriver.dll 同目录运行 starriver-launcher.exe，打印 VERSION/DEVICE-PROFILE。
 * 内存契约注意：DLL 返回串由 DLL 内 mingw CRT malloc 分配；exe 侧 CRT 是另一个堆，
 * 跨模块 free 属未定义行为，故本启动器不 free（打印后进程即退出）。
 * M5 裁决：DLL 增 SrFree 导出或统一 CRT 后再回收。Linux 侧 ASan/Valgrind 无法覆盖本文件（DEFERRED）。
 */
#include <windows.h>
#include <stdio.h>

typedef char *(*sr_str_fn)(void);

static int print_export(HMODULE dll, const char *name, const char *label) {
    /* FARPROC -> 函数指针经 void(*)(void) 中转，避开 -Wcast-function-type */
    sr_str_fn fn = (sr_str_fn)(void (*)(void))GetProcAddress(dll, name);
    if (!fn) {
        fprintf(stderr, "[FAIL] export %s not found: %lu\n", name, GetLastError());
        return 1;
    }
    char *s = fn();
    if (!s) {
        fprintf(stderr, "[FAIL] %s returned NULL\n", name);
        return 1;
    }
    printf("%s: %s\n", label, s);
    return 0;
}

int main(void) {
    HMODULE dll = LoadLibraryA("starriver.dll");
    if (!dll) {
        fprintf(stderr, "[FAIL] LoadLibrary starriver.dll: %lu\n", GetLastError());
        return 1;
    }
    int fail = 0;
    fail |= print_export(dll, "SrVersion", "VERSION");
    fail |= print_export(dll, "SrDeviceProfile", "DEVICE-PROFILE");
    FreeLibrary(dll);
    if (fail) return 1;
    printf("LAUNCHER-OK\n");
    return 0;
}
