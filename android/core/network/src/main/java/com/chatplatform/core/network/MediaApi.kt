package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path

data class PresignUploadBody(val kind: String, val mime_type: String, val file_name: String? = null)
data class PresignUploadResponseDto(val media_id: String, val upload_url: String, val expires_in_seconds: Int)
data class ConfirmUploadResponseDto(val media_id: String, val status: String, val download_url: String?, val reason: String?)
data class DownloadUrlResponseDto(val download_url: String, val expires_in_seconds: Int)

interface MediaApi {
    @POST("v1/media/presign-upload")
    suspend fun presignUpload(@Body body: PresignUploadBody): PresignUploadResponseDto

    @POST("v1/media/{mediaId}/confirm")
    suspend fun confirmUpload(@Path("mediaId") mediaId: String): ConfirmUploadResponseDto

    @GET("v1/media/{mediaId}/download-url")
    suspend fun getDownloadUrl(@Path("mediaId") mediaId: String): DownloadUrlResponseDto
}
