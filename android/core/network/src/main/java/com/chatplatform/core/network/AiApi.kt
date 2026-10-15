package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.Path

data class CreateAiConversationBody(val title: String? = null)
data class UpdateAiConversationBody(val title: String)
data class AiConversationDto(
    val id: String,
    val user_id: String,
    val title: String,
    val model: String,
    val created_at: String,
    val updated_at: String,
)
data class AiConversationsResponseDto(val conversations: List<AiConversationDto>?)
data class AiMessageDto(
    val id: String,
    val conversation_id: String,
    val role: String, // "user" | "model"
    val content: String,
    val finish_reason: String?,
    val error: String?,
    val created_at: String,
)
data class AiMessagesResponseDto(val messages: List<AiMessageDto>?)

/**
 * The CRUD side of the AI agent — everything except actually sending a
 * message, which is a Server-Sent Events stream and is handled by
 * [AiStreamClient] with a raw OkHttp call instead (Retrofit's suspend
 * functions aren't a fit for "keep reading chunks as they arrive").
 */
interface AiApi {
    @POST("v1/ai/conversations")
    suspend fun createConversation(@Body body: CreateAiConversationBody): AiConversationDto

    @GET("v1/ai/conversations")
    suspend fun listConversations(): AiConversationsResponseDto

    @GET("v1/ai/conversations/{conversationId}")
    suspend fun getConversation(@Path("conversationId") conversationId: String): AiConversationDto

    @PATCH("v1/ai/conversations/{conversationId}")
    suspend fun renameConversation(@Path("conversationId") conversationId: String, @Body body: UpdateAiConversationBody)

    @DELETE("v1/ai/conversations/{conversationId}")
    suspend fun deleteConversation(@Path("conversationId") conversationId: String)

    @GET("v1/ai/conversations/{conversationId}/messages")
    suspend fun listMessages(@Path("conversationId") conversationId: String): AiMessagesResponseDto
}
