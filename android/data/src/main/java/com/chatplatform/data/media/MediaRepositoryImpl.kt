package com.chatplatform.data.media

import com.chatplatform.core.network.MediaApi
import com.chatplatform.core.network.MediaUploader
import com.chatplatform.core.network.PresignUploadBody
import com.chatplatform.domain.media.MediaKind
import com.chatplatform.domain.media.MediaRepository
import com.chatplatform.domain.media.PendingUpload
import com.chatplatform.domain.media.UploadResult

class MediaRepositoryImpl(
    private val api: MediaApi,
    private val uploader: MediaUploader,
) : MediaRepository {

    override suspend fun upload(pending: PendingUpload, onProgress: (Long, Long) -> Unit): UploadResult {
        val presign = try {
            api.presignUpload(
                PresignUploadBody(
                    kind = pending.kind.name.lowercase(),
                    mime_type = pending.mimeType,
                    file_name = pending.fileName,
                )
            )
        } catch (e: Exception) {
            return UploadResult.Failed("Could not start upload: ${e.message}")
        }

        val putResult = uploader.upload(
            uploadUrl = presign.upload_url,
            contentType = pending.mimeType,
            totalBytes = pending.sizeBytes,
            openStream = pending.openStream,
            onProgress = onProgress,
        )
        if (putResult.isFailure) {
            return UploadResult.Failed("Upload to storage failed: ${putResult.exceptionOrNull()?.message}")
        }

        return try {
            val confirmed = api.confirmUpload(presign.media_id)
            when (confirmed.status) {
                "uploaded" -> UploadResult.Success(confirmed.media_id)
                "rejected" -> UploadResult.Rejected(confirmed.reason ?: "File was rejected by the server")
                else -> UploadResult.Failed("Unexpected status: ${confirmed.status}")
            }
        } catch (e: Exception) {
            UploadResult.Failed("Could not confirm upload: ${e.message}")
        }
    }

    override suspend fun getDownloadUrl(mediaId: String): String =
        api.getDownloadUrl(mediaId).download_url
}
