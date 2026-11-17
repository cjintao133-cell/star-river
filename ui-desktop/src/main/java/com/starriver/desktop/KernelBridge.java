package com.starriver.desktop;

/**
 * JNI 桥（T-M0-05 spike：Java 载体验证 JNI ABI；M5 Compose 工程换 Kotlin object，
 * 包名/类名/方法名不变则 JNI 符号不变）。库由调用方 System.load 绝对路径加载。
 */
public final class KernelBridge {
    public static native String version();
    public static native String echo(String s);
    private KernelBridge() {}
}
