import com.starriver.desktop.KernelBridge;

/** T-M5-03 桌面 JNI 全链路无头冒烟：init→角色→会话→消息→列表→删除。
 *  含中文与 emoji(补充平面)，验证 byte[] UTF-8 桥的无损往返。 */
public class DesktopKernelSmoke {
    static void eq(String w, String g, String n) { if (!w.equals(g)) throw new AssertionError(n+": got ["+g+"] want ["+w+"]"); System.out.println("[OK] "+n); }
    static void has(String h, String s, String n) { if (h==null || !h.contains(s)) throw new AssertionError(n+": missing ["+s+"] in ["+h+"]"); System.out.println("[OK] "+n); }
    public static void main(String[] a) throws Exception {
        System.load(a[0]); String dir = a[1]; String key = a[2];
        eq("OK", KernelBridge.initKernel(dir+"/smoke.db", key), "initKernel");
        eq("OK", KernelBridge.createCharacter("{\"id\":\"c-smoke\",\"name\":\"示例角色\"}"), "createCharacter");
        has(KernelBridge.listCharacters(), "示例角色", "listCharacters 中文");
        String sj = KernelBridge.createSession("c-smoke", "测试会话", "deepseek-flash");
        has(sj, "session_id", "createSession");
        String sid = sj.replaceAll(".*\"session_id\":\"([^\"]+)\".*", "$1");
        eq("OK", KernelBridge.appendMessage(sid, "user", "你好🌌世界"), "appendMessage emoji");
        eq("OK", KernelBridge.appendMessage(sid, "assistant", "哼 "), "appendMessage assistant");
        String msgs = KernelBridge.listMessages(sid, 50);
        has(msgs, "你好🌌世界", "listMessages emoji 无损");
        has(KernelBridge.listSessions(), "测试会话", "listSessions 中文");
        eq("OK", KernelBridge.deleteSession(sid), "deleteSession");
        System.out.println("DESKTOP-JNI-KERNEL-GREEN");
    }
}
