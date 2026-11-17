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
        System.out.println("JNI-LINUX-GREEN");
    }

    private static void expect(String want, String got, String name) {
        if (!want.equals(got)) {
            throw new AssertionError(name + ": got [" + got + "] want [" + want + "]");
        }
        System.out.println("[OK] " + name);
    }
}
