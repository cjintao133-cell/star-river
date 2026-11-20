/* JNI 薄桥（T-M5-03 全功能接入）：KernelBridge 会话/角色/单次聊天 native → Go c导出。
 * 编码：全部经 jbyteArray 承载 UTF-8（不走 NewStringUTF），规避 Modified UTF-8 对
 * emoji/补充平面的破坏（红线见 docs/m0-conclusions.md T-M0-05）。
 * 内存契约：Go 返回 malloc 字符串，本层拷入 jbyteArray 后 free；入参 jbyteArray 拷成
 * 临时 C 串，调用后 free。
 * 加固（按 astra 增量评审 P1）：
 *   - jb2c 处理 null/空/分配失败，GetByteArrayRegion 后查 ExceptionCheck；
 *   - c2jb 校验长度不超 INT32_MAX，NewByteArray 失败不再写、恰好 free 一次；
 *   - 多参函数任一转换失败即释放已分配并返回 "ERR:jni-oom"，绝不把 NULL 传入 Go。
 * REVIEW-NEEDED：手写 cgo/JNI 指针边界（红线 7）。jnibridge.c 挂起评审中，本文件独立新增。
 */
#include <jni.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

extern char* SrInitKernel(char* dbPath, char* keyB64);
extern char* SrListSessions(void);
extern char* SrCreateSession(char* characterID, char* title, char* model);
extern char* SrListMessages(char* sessionID, int limit);
extern char* SrAppendMessage(char* sessionID, char* role, char* text);
extern char* SrDeleteSession(char* sessionID);
extern char* SrListCharacters(void);
extern char* SrCreateCharacter(char* json);
extern char* SrDeleteCharacter(char* id);
extern char* SrChatOnce(char* base, char* apiKey, char* model, char* system, char* user);

/* jbyteArray(UTF-8) → malloc C 串（NUL 结尾）。NULL 入参→NULL；分配失败/取字节异常→NULL。 */
static char* jb2c(JNIEnv* env, jbyteArray a)
{
    if (a == NULL) return NULL;
    jsize n = (*env)->GetArrayLength(env, a);
    if (n < 0) return NULL;
    size_t sz = (size_t)n + 1;
    if (sz < (size_t)n) return NULL; /* size_t 溢出兜底 */
    char* buf = (char*)malloc(sz);
    if (buf == NULL) return NULL;
    if (n > 0) {
        (*env)->GetByteArrayRegion(env, a, 0, n, (jbyte*)buf);
        if ((*env)->ExceptionCheck(env)) { free(buf); return NULL; } /* 保留异常，交由 JVM 传播 */
    }
    buf[n] = '\0';
    return buf;
}

/* 由静态串造错误 jbyteArray（malloc 临时串后交给 c2jb 释放）。 */
static jbyteArray errj(JNIEnv* env, const char* m)
{
    size_t n = strlen(m);
    char* c = (char*)malloc(n + 1);
    if (c == NULL) return NULL;
    memcpy(c, m, n + 1);
    jbyteArray out = NULL;
    jsize jn = (jsize)n;
    out = (*env)->NewByteArray(env, jn);
    if (out != NULL) (*env)->SetByteArrayRegion(env, out, 0, jn, (const jbyte*)c);
    free(c);
    return out;
}

/* Go malloc C 串 → jbyteArray，并恰好 free 一次。NULL→NULL；超长/NewByteArray 失败→释放后 NULL。 */
static jbyteArray c2jb(JNIEnv* env, char* s)
{
    if (s == NULL) return NULL;
    size_t len = strlen(s);
    if (len > (size_t)INT32_MAX) { free(s); return NULL; }
    jsize n = (jsize)len;
    jbyteArray out = (*env)->NewByteArray(env, n);
    if (out == NULL) { free(s); return NULL; }
    if (n > 0) (*env)->SetByteArrayRegion(env, out, 0, n, (const jbyte*)s);
    free(s);
    return out;
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nInitKernel(JNIEnv* env, jclass cls, jbyteArray db, jbyteArray key)
{
    (void)cls;
    char* cdb = jb2c(env, db);
    char* ckey = jb2c(env, key);
    if (cdb == NULL || ckey == NULL) { free(cdb); free(ckey); return errj(env, "ERR:jni-oom"); }
    char* out = SrInitKernel(cdb, ckey);
    free(cdb); free(ckey);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nListSessions(JNIEnv* env, jclass cls)
{
    (void)cls;
    return c2jb(env, SrListSessions());
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nCreateSession(JNIEnv* env, jclass cls, jbyteArray characterID, jbyteArray title, jbyteArray model)
{
    (void)cls;
    char* a = jb2c(env, characterID); /* 允许空串表示无角色 */
    char* b = jb2c(env, title);
    char* c = jb2c(env, model);
    if ((characterID != NULL && a == NULL) || b == NULL || c == NULL) { free(a); free(b); free(c); return errj(env, "ERR:jni-oom"); }
    char* out = SrCreateSession(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nListMessages(JNIEnv* env, jclass cls, jbyteArray sid, jint limit)
{
    (void)cls;
    char* a = jb2c(env, sid);
    if (a == NULL) { return errj(env, "ERR:jni-oom"); }
    char* out = SrListMessages(a, (int)limit);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nAppendMessage(JNIEnv* env, jclass cls, jbyteArray sid, jbyteArray role, jbyteArray text)
{
    (void)cls;
    char* a = jb2c(env, sid);
    char* b = jb2c(env, role);
    char* c = jb2c(env, text); /* 允许空文本 */
    if (a == NULL || b == NULL || (text != NULL && c == NULL)) { free(a); free(b); free(c); return errj(env, "ERR:jni-oom"); }
    char* out = SrAppendMessage(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteSession(JNIEnv* env, jclass cls, jbyteArray sid)
{
    (void)cls;
    char* a = jb2c(env, sid);
    if (a == NULL) { return errj(env, "ERR:jni-oom"); }
    char* out = SrDeleteSession(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nListCharacters(JNIEnv* env, jclass cls)
{
    (void)cls;
    return c2jb(env, SrListCharacters());
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nCreateCharacter(JNIEnv* env, jclass cls, jbyteArray json)
{
    (void)cls;
    char* a = jb2c(env, json);
    if (a == NULL) { return errj(env, "ERR:jni-oom"); }
    char* out = SrCreateCharacter(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteCharacter(JNIEnv* env, jclass cls, jbyteArray id)
{
    (void)cls;
    char* a = jb2c(env, id);
    if (a == NULL) { return errj(env, "ERR:jni-oom"); }
    char* out = SrDeleteCharacter(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nChatOnce(JNIEnv* env, jclass cls, jbyteArray base, jbyteArray key, jbyteArray model, jbyteArray system, jbyteArray user)
{
    (void)cls;
    char* a = jb2c(env, base);
    char* b = jb2c(env, key);
    char* c = jb2c(env, model);
    char* d = jb2c(env, system);
    char* e = jb2c(env, user);
    if (a == NULL || c == NULL || e == NULL || (key != NULL && b == NULL) || (system != NULL && d == NULL)) {
        free(a); free(b); free(c); free(d); free(e); return errj(env, "ERR:jni-oom");
    }
    char* out = SrChatOnce(a, b, c, d, e);
    free(a); free(b); free(c); free(d); free(e);
    return c2jb(env, out);
}
