package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

data class StartConversationBody(val peer_user_id: String)
data class ConversationDto(
    val id: String,
    val type: String,
    val peer_user_id: String?,
    val name: String? = null, // group name; null for direct conversations
    val last_message: MessageDto?,
    val unread_count: Long,
)
data class ConversationsResponseDto(val conversations: List<ConversationDto>)

data class SendMessageBody(
    val client_tmp_id: String,
    val type: String,
    val body: String? = null,
    val reply_to_id: String? = null,
    val attachment_media_ids: List<String>? = null,
    val forwarded_from_message_id: String? = null,
)
data class MessageDto(
    val id: String,
    val conversation_id: String,
    val seq: Long,
    val sender_id: String,
    val client_tmp_id: String,
    val type: String,
    val body: String?,
    val reply_to_id: String?,
    val created_at: String,
    val edited_at: String?,
    val attachment_ids: List<String>? = null,
    val forwarded_from_message_id: String? = null,
)
data class MessagePageDto(val messages: List<MessageDto>?, val next_cursor: String?)

data class ReactionBody(val emoji: String)
data class ReactionSummaryDto(val emoji: String, val user_ids: List<String>)
data class ReactionsResponseDto(val reactions: List<ReactionSummaryDto>?)

data class SearchResponseDto(val messages: List<MessageDto>?)

data class MuteBody(val until: String? = null)

interface ChatApi {
    @GET("v1/conversations")
    suspend fun listConversations(): ConversationsResponseDto

    @POST("v1/conversations")
    suspend fun startConversation(@Body body: StartConversationBody): ConversationDto

    @GET("v1/conversations/{conversationId}/messages")
    suspend fun listMessages(
        @Path("conversationId") conversationId: String,
        @Query("cursor") cursor: String? = null,
    ): MessagePageDto

    @POST("v1/conversations/{conversationId}/messages")
    suspend fun sendMessageRest(
        @Path("conversationId") conversationId: String,
        @Body body: SendMessageBody,
    ): MessageDto

    @retrofit2.http.PUT("v1/conversations/{conversationId}/mute")
    suspend fun setMute(@Path("conversationId") conversationId: String, @Body body: MuteBody)

    @POST("v1/messages/{messageId}/reactions")
    suspend fun setReaction(@Path("messageId") messageId: String, @Body body: ReactionBody)

    @DELETE("v1/messages/{messageId}/reactions")
    suspend fun removeReaction(@Path("messageId") messageId: String)

    @GET("v1/messages/{messageId}/reactions")
    suspend fun listReactions(@Path("messageId") messageId: String): ReactionsResponseDto

    @GET("v1/search/messages")
    suspend fun searchMessages(@Query("q") query: String, @Query("conversation_id") conversationId: String? = null): SearchResponseDto
}
