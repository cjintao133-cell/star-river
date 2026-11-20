/* JNI 薄桥（T-M5-03 全功能接入）：KernelBridge 会话/角色/单次聊天 native → Go c导出。
 * 编码：全部经 jbyteArray 承载 UTF-8（不走 NewStringUTF），规避 Modified UTF-8 破坏 emoji。
 * 内存契约：Go 返回 malloc 字符串，本层拷入 jbyteArray 后 free；入参 jbyteArray 拷成
 * 临时 C 串，调用后 free。
 *
 * 加固（按 astra 增量评审 P1）：
 *   - jb2c 处理 null/空/分配失败，GetByteArrayRegion 后查 ExceptionCheck；
 *   - 拒绝内嵌 NUL（避免静默截断路径/ID/密钥）；
 *   - 有 pending 异常时绝不再调用任何 JNI 分配（errj），直接释放并返回 NULL 让异常传播；
 *   - 密钥类入参（key）释放前 secure_zero 擦除，防进程内存 dump 残留明文。
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

/* 转换状态码。 */
enum { JB_OK = 0, JB_OOM = 1, JB_EXC = 2, JB_NUL = 3 };

/* secure_zero 不可优化清零（密钥释放前用）。 */
static void secure_zero(void* p, size_t n)
{
    volatile unsigned char* v = (volatile unsigned char*)p;
    while (n--) *v++ = 0;
}

/* jbyteArray(UTF-8) → malloc C 串（NUL 结尾）。
 * a==NULL → 返回 NULL 且 *st=JB_OK（表示"缺省/空"，由调用方决定是否允许为空）。
 * 分配失败→JB_OOM；取字节异常→JB_EXC（异常已挂，调用方须直接返回不再调 JNI）；
 * 含内嵌 NUL→JB_NUL。成功返回非 NULL。 */
static char* jb2c(JNIEnv* env, jbyteArray a, int* st)
{
    *st = JB_OK;
    if (a == NULL) return NULL;
    jsize n = (*env)->GetArrayLength(env, a);
    if (n < 0) { *st = JB_EXC; return NULL; }
    size_t sz = (size_t)n + 1;
    if (sz < (size_t)n) { *st = JB_OOM; return NULL; }
    char* buf = (char*)malloc(sz);
    if (buf == NULL) { *st = JB_OOM; return NULL; }
    if (n > 0) {
        (*env)->GetByteArrayRegion(env, a, 0, n, (jbyte*)buf);
        if ((*env)->ExceptionCheck(env)) { secure_zero(buf, sz); free(buf); *st = JB_EXC; return NULL; }
        for (jsize i = 0; i < n; i++) {
            if (buf[i] == '\0') { secure_zero(buf, sz); free(buf); *st = JB_NUL; return NULL; }
        }
    }
    buf[n] = '\0';
    return buf;
}

/* empty_dup 返回 malloc 的空串（缺省字段归一为空串）。失败返回 NULL。 */
static char* empty_dup(void)
{
    char* c = (char*)malloc(1);
    if (c) c[0] = '\0';
    return c;
}

/* 由静态串造错误 jbyteArray（malloc 临时串后释放）。仅在无 pending 异常时调用。 */
static jbyteArray errj(JNIEnv* env, const char* m)
{
    size_t n = strlen(m);
    char* c = (char*)malloc(n + 1);
    if (c == NULL) return NULL;
    memcpy(c, m, n + 1);
    jsize jn = (jsize)n;
    jbyteArray out = (*env)->NewByteArray(env, jn);
    if (out != NULL) (*env)->SetByteArrayRegion(env, out, 0, jn, (const jbyte*)c);
    free(c);
    return out;
}

/* Go malloc C 串 → jbyteArray，并恰好 free 一次。NULL→NULL；超长/失败→释放后 NULL。 */
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

/* 收尾：若已挂异常，安全释放并返回 NULL（不调 errj）；否则按 bad 返回错误串。 */
static jbyteArray finish_err(JNIEnv* env, int has_exc, char* a, char* b, char* c, char* d, char* e)
{
    secure_zero(a, a ? strlen(a) : 0); free(a);
    secure_zero(b, b ? strlen(b) : 0); free(b);
    secure_zero(c, c ? strlen(c) : 0); free(c);
    secure_zero(d, d ? strlen(d) : 0); free(d);
    secure_zero(e, e ? strlen(e) : 0); free(e);
    if (has_exc) return NULL; /* 异常已挂，交由 JVM 传播 */
    return errj(env, "ERR:jni-badarg");
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nInitKernel(JNIEnv* env, jclass cls, jbyteArray db, jbyteArray key)
{
    (void)cls;
    int sd = JB_OK, sk = JB_OK;
    char* cdb = jb2c(env, db, &sd);
    char* ckey = jb2c(env, key, &sk);
    if ((*env)->ExceptionCheck(env) || sd == JB_EXC || sk == JB_EXC) {
        return finish_err(env, 1, cdb, ckey, NULL, NULL, NULL);
    }
    if (sd != JB_OK || sk != JB_OK || cdb == NULL || ckey == NULL) {
        return finish_err(env, 0, cdb, ckey, NULL, NULL, NULL);
    }
    char* out = SrInitKernel(cdb, ckey);
    secure_zero(ckey, strlen(ckey)); /* #5 密钥擦除 */
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
    int s1 = JB_OK, s2 = JB_OK, s3 = JB_OK;
    char* a = jb2c(env, characterID, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    if (characterID == NULL) { free(a); a = empty_dup(); } /* characterID 允许缺省→空串 */
    char* b = jb2c(env, title, &s2);
    if (s2 == JB_EXC) return finish_err(env, 1, a, b, NULL, NULL, NULL);
    if (title == NULL) { free(b); b = empty_dup(); }
    char* c = jb2c(env, model, &s3);
    if (s3 == JB_EXC) return finish_err(env, 1, a, b, c, NULL, NULL);
    if (model == NULL) { free(c); c = empty_dup(); }
    if (s1 != JB_OK || s2 != JB_OK || s3 != JB_OK || a == NULL || b == NULL || c == NULL) {
        return finish_err(env, 0, a, b, c, NULL, NULL);
    }
    char* out = SrCreateSession(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nListMessages(JNIEnv* env, jclass cls, jbyteArray sid, jint limit)
{
    (void)cls;
    int s1 = JB_OK;
    char* a = jb2c(env, sid, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    if (s1 != JB_OK || a == NULL) return finish_err(env, 0, a, NULL, NULL, NULL, NULL);
    char* out = SrListMessages(a, (int)limit);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nAppendMessage(JNIEnv* env, jclass cls, jbyteArray sid, jbyteArray role, jbyteArray text)
{
    (void)cls;
    int s1 = JB_OK, s2 = JB_OK, s3 = JB_OK;
    char* a = jb2c(env, sid, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    char* b = jb2c(env, role, &s2);
    if (s2 == JB_EXC) return finish_err(env, 1, a, b, NULL, NULL, NULL);
    char* c = jb2c(env, text, &s3);
    if (s3 == JB_EXC) return finish_err(env, 1, a, b, c, NULL, NULL);
    if (text == NULL) { free(c); c = empty_dup(); } /* text 允许缺省→空串 */
    if (s1 != JB_OK || s2 != JB_OK || s3 != JB_OK || a == NULL || b == NULL || c == NULL) {
        return finish_err(env, 0, a, b, c, NULL, NULL);
    }
    char* out = SrAppendMessage(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteSession(JNIEnv* env, jclass cls, jbyteArray sid)
{
    (void)cls;
    int s1 = JB_OK;
    char* a = jb2c(env, sid, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    if (s1 != JB_OK || a == NULL) return finish_err(env, 0, a, NULL, NULL, NULL, NULL);
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
    int s1 = JB_OK;
    char* a = jb2c(env, json, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    if (s1 != JB_OK || a == NULL) return finish_err(env, 0, a, NULL, NULL, NULL, NULL);
    char* out = SrCreateCharacter(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteCharacter(JNIEnv* env, jclass cls, jbyteArray id)
{
    (void)cls;
    int s1 = JB_OK;
    char* a = jb2c(env, id, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    if (s1 != JB_OK || a == NULL) return finish_err(env, 0, a, NULL, NULL, NULL, NULL);
    char* out = SrDeleteCharacter(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nChatOnce(JNIEnv* env, jclass cls, jbyteArray base, jbyteArray key, jbyteArray model, jbyteArray system, jbyteArray user)
{
    (void)cls;
    int s1 = JB_OK, s2 = JB_OK, s3 = JB_OK, s4 = JB_OK, s5 = JB_OK;
    char* a = jb2c(env, base, &s1);
    if (s1 == JB_EXC) return finish_err(env, 1, a, NULL, NULL, NULL, NULL);
    char* b = jb2c(env, key, &s2);
    if (s2 == JB_EXC) return finish_err(env, 1, a, b, NULL, NULL, NULL);
    if (key == NULL) { free(b); b = empty_dup(); }
    char* c = jb2c(env, model, &s3);
    if (s3 == JB_EXC) return finish_err(env, 1, a, b, c, NULL, NULL);
    char* d = jb2c(env, system, &s4);
    if (s4 == JB_EXC) return finish_err(env, 1, a, b, c, d, NULL);
    if (system == NULL) { free(d); d = empty_dup(); }
    char* e = jb2c(env, user, &s5);
    if (s5 == JB_EXC) return finish_err(env, 1, a, b, c, d, e);
    if (s1 != JB_OK || s2 != JB_OK || s3 != JB_OK || s4 != JB_OK || s5 != JB_OK ||
        a == NULL || b == NULL || c == NULL || d == NULL || e == NULL) {
        return finish_err(env, 0, a, b, c, d, e);
    }
    char* out = SrChatOnce(a, b, c, d, e);
    secure_zero(b, strlen(b)); /* #5 密钥擦除 */
    free(a); free(b); free(c); free(d); free(e);
    return c2jb(env, out);
}
