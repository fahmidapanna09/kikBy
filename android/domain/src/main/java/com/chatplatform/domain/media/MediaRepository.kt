package com.chatplatform.domain.media

enum class MediaKind { IMAGE, VIDEO, AUDIO, VOICE, DOCUMENT }

data class PendingUpload(
    val kind: MediaKind,
    val mimeType: String,
    val fileName: String?,
    val sizeBytes: Long,
    val openStream: () -> java.io.InputStream,
)

sealed class UploadResult {
    data class Success(val mediaId: String) : UploadResult()
    data class Rejected(val reason: String) : UploadResult()
    data class Failed(val message: String) : UploadResult()
}

interface MediaRepository {
    /** Runs the full presign -> PUT -> confirm sequence. Does not touch
     *  chat/messages at all — the caller attaches the returned mediaId to a
     *  message via ChatRepository.sendMessage's attachmentMediaIds param. */
    suspend fun upload(pending: PendingUpload, onProgress: (sent: Long, total: Long) -> Unit = { _, _ -> }): UploadResult

    /** Presigned URLs expire — always fetch a fresh one right before use
     *  (e.g. right before handing a URL to an image loader or player)
     *  rather than caching it long-term. */
    suspend fun getDownloadUrl(mediaId: String): String
}
