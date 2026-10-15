package com.chatplatform.domain.chat

import kotlinx.coroutines.flow.Flow

enum class MessageDeliveryStatus { PENDING, SENT, DELIVERED, READ, FAILED }

data class ChatMessage(
    val clientTmpId: String,
    val serverId: String?,
    val conversationId: String,
    val senderId: String,
    val type: String,
    val body: String?,
    val replyToId: String?,
    val seq: Long?,
    val createdAtEpochMs: Long,
    val status: MessageDeliveryStatus,
    val attachmentIds: List<String> = emptyList(),
    val forwardedFromMessageId: String? = null,
)

data class ReactionSummary(val emoji: String, val userIds: List<String>)

data class Conversation(
    val id: String,
    val type: String,
    val peerUserId: String?,
    /** Group name for type == "group"; null for direct conversations
     *  (resolve those via ContactNameProvider using peerUserId). */
    val name: String? = null,
    val lastMessage: ChatMessage?,
    val unreadCount: Long,
)

enum class ConnectionState { CONNECTING, CONNECTED, DISCONNECTED }

interface ChatRepository {
    /** Local-first: emits from the Room cache immediately, and again
     *  whenever new data arrives from the network or WebSocket. */
    fun observeConversations(): Flow<List<Conversation>>
    fun observeMessages(conversationId: String): Flow<List<ChatMessage>>
    val connectionState: Flow<ConnectionState>

    /** Keyed by message *server* id (reactions on a still-PENDING,
     *  not-yet-acked message won't appear here — there's nothing to react
     *  to server-side yet). Updated live from the message.reaction WS event;
     *  not persisted to Room, since it's cheap to re-fetch/re-derive and
     *  changes frequently. */
    fun observeReactions(): Flow<Map<String, List<ReactionSummary>>>

    suspend fun startConversation(peerUserId: String): Conversation
    suspend fun refreshConversations()
    suspend fun loadOlderMessages(conversationId: String)

    /** Writes the message to the local outbox immediately (optimistic UI)
     *  and returns right away; actual send happens async over WS/REST with
     *  retry. Call connectAndStartSyncing() once at app start / login.
     *  attachmentMediaIds must already be confirmed-uploaded media ids
     *  (see MediaRepository.upload) — this call does not upload anything
     *  itself, it only references media that's already on the server.
     *  forwardedFromMessageId is provenance only (see backend docs) — pass
     *  the original message's serverId when forwarding; body/attachments
     *  must still be supplied by the caller, copied from the original. */
    suspend fun sendMessage(
        conversationId: String,
        clientTmpId: String,
        type: String,
        body: String?,
        replyToId: String? = null,
        attachmentMediaIds: List<String> = emptyList(),
        forwardedFromMessageId: String? = null,
    )

    suspend fun markDelivered(conversationId: String, messageServerId: String)
    suspend fun markRead(conversationId: String, messageServerId: String)
    suspend fun sendTyping(conversationId: String, isTyping: Boolean)

    suspend fun setReaction(messageServerId: String, emoji: String)
    suspend fun removeReaction(messageServerId: String)

    /** conversationId null = search across every conversation the user is
     *  currently a member of (server-enforced, not client-filtered). */
    suspend fun searchMessages(query: String, conversationId: String? = null): List<ChatMessage>

    fun connectAndStartSyncing()
    fun disconnect()
}
