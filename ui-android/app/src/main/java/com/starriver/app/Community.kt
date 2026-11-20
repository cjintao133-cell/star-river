package com.starriver.app

import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

/**
 * 社区生态拉取：Git 仓库（或任何静态托管）当后端，客户端只做 HTTPS GET 拉 JSON。
 * 契约见 docs/community-ecosystem-contract.md。失败一律回落（返回 null），不崩、不白屏。
 */
object Community {
    data class CatalogPlugin(
        val id: String, val name: String, val description: String,
        val transport: String, val endpoint: String,
        val authType: String, val authHeader: String, val authHint: String,
        val categories: List<String>,
    )

    data class ForumPost(
        val id: String, val title: String, val author: String,
        val replies: Int, val tags: List<String>, val body: String,
    )

    private fun base(): String = CommunityPrefs.base(AppCtx.ctx)

    private fun fetch(path: String): String? {
        val b = base().trim().trimEnd('/')
        if (b.isEmpty()) return null
        val url = if (b.endsWith("/")) b + path else "$b/$path"
        if (!url.startsWith("https://")) return null // 只走 HTTPS
        var conn: HttpURLConnection? = null
        return try {
            conn = (URL(url).openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = 8000
                readTimeout = 8000
                setRequestProperty("Accept", "application/json")
            }
            val code = conn.responseCode
            if (code !in 200..299) return null
            conn.inputStream.bufferedReader().use { it.readText() }
        } catch (t: Throwable) {
            null
        } finally {
            conn?.disconnect()
        }
    }

    /** 拉市场目录；失败返回 null（UI 回落种子）。 */
    fun fetchCatalog(): List<CatalogPlugin>? {
        val raw = fetch("market/index.json") ?: return null
        return try {
            val arr = JSONObject(raw).optJSONArray("plugins") ?: return emptyList()
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.optJSONObject(i) ?: return@mapNotNull null
                val auth = o.optJSONObject("auth")
                val cats = mutableListOf<String>()
                o.optJSONArray("categories")?.let { c -> for (j in 0 until c.length()) cats.add(c.optString(j)) }
                CatalogPlugin(
                    id = o.optString("id"),
                    name = o.optString("name").ifBlank { o.optString("id") },
                    description = o.optString("description"),
                    transport = o.optString("transport", "http"),
                    endpoint = o.optString("endpoint"),
                    authType = auth?.optString("type", "none") ?: "none",
                    authHeader = auth?.optString("header", "Authorization") ?: "Authorization",
                    authHint = auth?.optString("hint", "") ?: "",
                    categories = cats,
                )
            }
        } catch (e: Exception) { null }
    }

    /** 拉论坛帖子；失败返回 null（UI 回落种子）。 */
    fun fetchPosts(): List<ForumPost>? {
        val raw = fetch("forum/posts.json") ?: return null
        return try {
            val arr = JSONObject(raw).optJSONArray("posts") ?: return emptyList()
            (0 until arr.length()).mapNotNull { i ->
                val o = arr.optJSONObject(i) ?: return@mapNotNull null
                val tags = mutableListOf<String>()
                o.optJSONArray("tags")?.let { t -> for (j in 0 until t.length()) tags.add(t.optString(j)) }
                ForumPost(
                    id = o.optString("id"),
                    title = o.optString("title"),
                    author = o.optString("author"),
                    replies = o.optInt("replies", 0),
                    tags = tags,
                    body = o.optString("body"),
                )
            }
        } catch (e: Exception) { null }
    }
}
