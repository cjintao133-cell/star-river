package com.starriver.desktop;

import java.nio.charset.StandardCharsets;

/**
 * JNI 桥（T-M0-05 spike + T-M5-03 全功能接入）。
 * 包名/类名/方法名不变则 JNI 符号不变。库由调用方 System.load 绝对路径加载。
 *
 * version/echo/deviceProfile 历史方法走 NewStringUTF（ASCII/受控 JSON）。
 * T-M5-03 新增会话/角色/单次聊天一律走 byte[]（UTF-8）避免 Modified UTF-8 破坏任意文本；
 * 对外暴露 String 便捷包装（内部转 UTF-8 byte[]）。
 */
public final class KernelBridge {
    // —— 历史（spike）——
    public static native String version();
    public static native String echo(String s);
    public static native String deviceProfile();

    // —— T-M5-03 全功能接入原生层（UTF-8 byte[]）——
    private static native byte[] nInitKernel(byte[] dbPath, byte[] keyB64);
    private static native byte[] nListSessions();
    private static native byte[] nCreateSession(byte[] characterID, byte[] title, byte[] model);
    private static native byte[] nListMessages(byte[] sessionID, int limit);
    private static native byte[] nAppendMessage(byte[] sessionID, byte[] role, byte[] text);
    private static native byte[] nDeleteSession(byte[] sessionID);
    private static native byte[] nListCharacters();
    private static native byte[] nCreateCharacter(byte[] json);
    private static native byte[] nDeleteCharacter(byte[] id);
    private static native byte[] nChatOnce(byte[] base, byte[] key, byte[] model, byte[] system, byte[] user);

    private static byte[] u(String s) { return s == null ? null : s.getBytes(StandardCharsets.UTF_8); }
    private static String s(byte[] b) { return b == null ? null : new String(b, StandardCharsets.UTF_8); }

    // —— String 便捷包装 ——
    public static String initKernel(String dbPath, String keyB64) { return s(nInitKernel(u(dbPath), u(keyB64))); }
    public static String listSessions() { return s(nListSessions()); }
    public static String createSession(String characterID, String title, String model) { return s(nCreateSession(u(characterID), u(title), u(model))); }
    public static String listMessages(String sessionID, int limit) { return s(nListMessages(u(sessionID), limit)); }
    public static String appendMessage(String sessionID, String role, String text) { return s(nAppendMessage(u(sessionID), u(role), u(text))); }
    public static String deleteSession(String sessionID) { return s(nDeleteSession(u(sessionID))); }
    public static String listCharacters() { return s(nListCharacters()); }
    public static String createCharacter(String json) { return s(nCreateCharacter(u(json))); }
    public static String deleteCharacter(String id) { return s(nDeleteCharacter(u(id))); }
    public static String chatOnce(String base, String key, String model, String system, String user) { return s(nChatOnce(u(base), u(key), u(model), u(system), u(user))); }

    private KernelBridge() {}
}
