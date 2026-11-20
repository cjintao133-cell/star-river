# T-M0-06: device_probe 三目标静态库（在仓库根目录执行）
# 用法: make -j4 -f native/Makefile all && make -f native/Makefile symbols
# 三目标 = PRD 验收清单（android-arm64 / windows-amd64 / linux-amd64），与环境协议构建矩阵一致。

NDK_HOME  ?= /root/android-sdk/ndk/26.3.11579264
NDK_BIN   := $(NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin
NDK_CC    := $(NDK_BIN)/aarch64-linux-android21-clang
NDK_AR    := $(NDK_BIN)/llvm-ar
NDK_STRIP := $(NDK_BIN)/llvm-strip

SRC := native/device_probe/device_probe.c
OUT := build/native

CFLAGS_COMMON := -O2 -Wall -Wextra -Werror -std=c11
# ELF 目标加隐藏可见性；PE/COFF 无此概念，靠 strip 达成同等效果
ELF_CFLAGS := $(CFLAGS_COMMON) -fvisibility=hidden

.PHONY: all linux android windows symbols

all: linux android windows

linux:
	mkdir -p $(OUT)/linux-amd64
	gcc $(ELF_CFLAGS) -c $(SRC) -o $(OUT)/linux-amd64/device_probe.o
	ar rcs $(OUT)/linux-amd64/libdevice_probe.a $(OUT)/linux-amd64/device_probe.o
	strip --strip-unneeded $(OUT)/linux-amd64/libdevice_probe.a

android:
	mkdir -p $(OUT)/android-arm64
	$(NDK_CC) $(ELF_CFLAGS) -c $(SRC) -o $(OUT)/android-arm64/device_probe.o
	$(NDK_AR) rcs $(OUT)/android-arm64/libdevice_probe.a $(OUT)/android-arm64/device_probe.o
	$(NDK_STRIP) --strip-unneeded $(OUT)/android-arm64/libdevice_probe.a

windows:
	mkdir -p $(OUT)/windows-amd64
	x86_64-w64-mingw32-gcc $(CFLAGS_COMMON) -c $(SRC) -o $(OUT)/windows-amd64/device_probe.o
	x86_64-w64-mingw32-gcc-ar rcs $(OUT)/windows-amd64/libdevice_probe.a $(OUT)/windows-amd64/device_probe.o
	x86_64-w64-mingw32-strip --strip-unneeded $(OUT)/windows-amd64/libdevice_probe.a

symbols:
	@echo "== linux-amd64 (defined) =="
	@nm --defined-only $(OUT)/linux-amd64/libdevice_probe.a
	@echo "== android-arm64 (defined) =="
	@nm --defined-only $(OUT)/android-arm64/libdevice_probe.a
	@echo "== windows-amd64 (defined) =="
	@x86_64-w64-mingw32-nm --defined-only $(OUT)/windows-amd64/libdevice_probe.a
	@echo "== linux .o visibility =="
	@readelf -s $(OUT)/linux-amd64/device_probe.o | grep -E 'GLOBAL|WEAK'

# ---- T-M3-01 vector: int8 量化向量检索（三目标静态库 + 桌面单测/基准） ----
VSRC := native/vector/vector.c
VOUT := build/native
VFLAGS := -O2 -Wall -Wextra -Werror -std=c11

vector-linux:
	mkdir -p $(VOUT)/linux-amd64
	gcc $(VFLAGS) -fvisibility=hidden -c $(VSRC) -o $(VOUT)/linux-amd64/vector.o
	ar rcs $(VOUT)/linux-amd64/libvector.a $(VOUT)/linux-amd64/vector.o
	strip --strip-unneeded $(VOUT)/linux-amd64/libvector.a

vector-android:
	mkdir -p $(VOUT)/android-arm64
	$(NDK_CC) $(VFLAGS) -fvisibility=hidden -c $(VSRC) -o $(VOUT)/android-arm64/vector.o
	$(NDK_AR) rcs $(VOUT)/android-arm64/libvector.a $(VOUT)/android-arm64/vector.o
	$(NDK_STRIP) --strip-unneeded $(VOUT)/android-arm64/libvector.a

vector-windows:
	mkdir -p $(VOUT)/windows-amd64
	x86_64-w64-mingw32-gcc $(VFLAGS) -c $(VSRC) -o $(VOUT)/windows-amd64/vector.o
	x86_64-w64-mingw32-gcc-ar rcs $(VOUT)/windows-amd64/libvector.a $(VOUT)/windows-amd64/vector.o
	x86_64-w64-mingw32-strip --strip-unneeded $(VOUT)/windows-amd64/libvector.a

vector: vector-linux vector-android vector-windows

vector-test:
	mkdir -p build
	gcc $(VFLAGS) $(VSRC) native/vector/test/test_vector.c -o build/vector-test -lm

bench-vector:
	mkdir -p build
	gcc $(VFLAGS) $(VSRC) native/vector/test/bench_vector.c -o build/bench-vector -lm

.PHONY: vector vector-linux vector-android vector-windows vector-test bench-vector
