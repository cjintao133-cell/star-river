package com.starriver.desktop;

/** JNI 链路无头验证（Linux .so）。用法：java ... com.starriver.desktop.Main <libstarriver.so 绝对路径> */
public final class Main {
    public static void main(String[] args) {
        System.load(args[0]);
        expect("star-river-kernel/0.0.1-m0", KernelBridge.version(), "version");
        expect("hello", KernelBridge.echo("hello"), "echo-ascii");
        expect("", KernelBridge.echo(""), "echo-empty");
        expect("中文与emoji🌌混排", KernelBridge.echo("中文与emoji🌌混排"), "echo-utf8");
        expect("line1\nline2\ttab", KernelBridge.echo("line1\nline2\ttab"), "echo-control");
        String profile = KernelBridge.deviceProfile();
        if (profile == null || !profile.startsWith("{") || !profile.endsWith("}")
                || !profile.contains("\"neon\":") || !profile.contains("\"avx2\":")
                || !profile.contains("\"avail_mem_bytes\":")
                || !profile.contains("\"version\":\"star-river-kernel/0.0.1-m0\"")) {
            throw new AssertionError("device-profile: bad JSON [" + profile + "]");
        }
        System.out.println("[OK] device-profile");
        System.out.println("DEVICE-PROFILE: " + profile);
        System.out.println("JNI-LINUX-GREEN");
    }

    private static void expect(String want, String got, String name) {
        if (!want.equals(got)) {
            throw new AssertionError(name + ": got [" + got + "] want [" + want + "]");
        }
        System.out.println("[OK] " + name);
    }
}
