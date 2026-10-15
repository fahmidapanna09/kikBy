package com.chatplatform.core.network

import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okio.BufferedSink
import okio.source
import java.io.InputStream

/**
 * PUTs a file directly to a presigned S3/MinIO URL. Uses a *separate*
 * OkHttpClient from the one used for API calls — this one must NOT attach
 * the app's Authorization header (that would break the presigned URL's own
 * signature-based auth) and does not need the access-token refresh
 * authenticator either.
 */
class MediaUploader(private val plainClient: OkHttpClient) {

    /**
     * @param openStream called (possibly more than once, if OkHttp retries)
     *   to get a fresh InputStream for the file — pass e.g.
     *   `{ contentResolver.openInputStream(uri)!! }` rather than an
     *   already-opened stream.
     * @param onProgress called with (bytesSent, totalBytes) on a background
     *   thread; hop to the main thread yourself if updating UI directly.
     */
    suspend fun upload(
        uploadUrl: String,
        contentType: String,
        totalBytes: Long,
        openStream: () -> InputStream,
        onProgress: (Long, Long) -> Unit = { _, _ -> },
    ): Result<Unit> = runCatching {
        val body = object : RequestBody() {
            override fun contentType() = contentType.toMediaTypeOrNull()
            override fun contentLength() = totalBytes
            override fun writeTo(sink: BufferedSink) {
                openStream().use { input ->
                    val source = input.source()
                    var sent = 0L
                    val chunkSize = 64L * 1024
                    var read: Long
                    while (source.read(sink.buffer, chunkSize).also { read = it } != -1L) {
                        sink.flush()
                        sent += read
                        onProgress(sent, totalBytes)
                    }
                }
            }
        }

        val request = Request.Builder().url(uploadUrl).put(body).build()
        plainClient.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                error("Upload failed: HTTP ${response.code}")
            }
        }
    }
}
