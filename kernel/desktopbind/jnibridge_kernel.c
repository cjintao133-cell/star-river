/* JNI 薄桥（T-M5-03 全功能接入）：KernelBridge 会话/角色/单次聊天 native → Go c导出。
 * 编码：全部经 jbyteArray 承载 UTF-8（不走 NewStringUTF），规避 Modified UTF-8 对
 * emoji/补充平面的破坏（红线见 docs/m0-conclusions.md T-M0-05）。
 * 内存契约：Go 返回 malloc 字符串，本层拷入 jbyteArray 后 free；入参 jbyteArray 拷成
 * 临时 C 串，调用后 free。
 * REVIEW-NEEDED：手写 cgo/JNI 指针边界（红线 7）。jnibridge.c 挂起评审中，本文件独立新增。
 */
#include <jni.h>
#include <stdlib.h>
#include <string.h>

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

/* jbyteArray(UTF-8) → malloc C 串（NUL 结尾）。NULL 入参→NULL。 */
static char* jb2c(JNIEnv* env, jbyteArray a)
{
    if (a == NULL) return NULL;
    jsize n = (*env)->GetArrayLength(env, a);
    char* buf = (char*)malloc((size_t)n + 1);
    if (buf == NULL) return NULL;
    (*env)->GetByteArrayRegion(env, a, 0, n, (jbyte*)buf);
    buf[n] = '\0';
    return buf;
}

/* Go malloc C 串 → jbyteArray，并 free 原串。 */
static jbyteArray c2jb(JNIEnv* env, char* s)
{
    if (s == NULL) return NULL;
    jsize n = (jsize)strlen(s);
    jbyteArray out = (*env)->NewByteArray(env, n);
    if (out != NULL) (*env)->SetByteArrayRegion(env, out, 0, n, (const jbyte*)s);
    free(s);
    return out;
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nInitKernel(JNIEnv* env, jclass cls, jbyteArray db, jbyteArray key)
{
    (void)cls;
    char* cdb = jb2c(env, db);
    char* ckey = jb2c(env, key);
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
    char* a = jb2c(env, characterID);
    char* b = jb2c(env, title);
    char* c = jb2c(env, model);
    char* out = SrCreateSession(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nListMessages(JNIEnv* env, jclass cls, jbyteArray sid, jint limit)
{
    (void)cls;
    char* a = jb2c(env, sid);
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
    char* c = jb2c(env, text);
    char* out = SrAppendMessage(a, b, c);
    free(a); free(b); free(c);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteSession(JNIEnv* env, jclass cls, jbyteArray sid)
{
    (void)cls;
    char* a = jb2c(env, sid);
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
    char* out = SrCreateCharacter(a);
    free(a);
    return c2jb(env, out);
}

JNIEXPORT jbyteArray JNICALL
Java_com_starriver_desktop_KernelBridge_nDeleteCharacter(JNIEnv* env, jclass cls, jbyteArray id)
{
    (void)cls;
    char* a = jb2c(env, id);
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
    char* out = SrChatOnce(a, b, c, d, e);
    free(a); free(b); free(c); free(d); free(e);
    return c2jb(env, out);
}
