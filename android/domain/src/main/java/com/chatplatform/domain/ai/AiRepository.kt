package com.chatplatform.domain.ai

import kotlinx.coroutines.flow.Flow

enum class AiRole { USER, MODEL }

data class AiMessage(
    val id: String,
    val conversationId: String,
    val role: AiRole,
    val content: String,
    val finishReason: String?,
    val error: String?,
    val createdAtEpochMs: Long,
)

data class AiConversation(
    val id: String,
    val title: String,
    val model: String,
    val createdAtEpochMs: Long,
    val updatedAtEpochMs: Long,
)

/** One increment of a streaming reply, as surfaced to the UI layer. */
sealed class AiTurnEvent {
    data class Delta(val text: String) : AiTurnEvent()
    data class Completed(val message: AiMessage) : AiTurnEvent()
    data class Failed(val reason: String) : AiTurnEvent()
}

class AiActionException(message: String) : Exception(message)

/**
 * Deliberately the narrowest repository interface in the app: it has no
 * way to reach any other feature's data (no chat, no contacts, nothing).
 * The AI agent only ever operates on what's explicitly sent to it through
 * [sendMessage] — see the backend's internal/ai/model.go package doc for
 * the matching server-side rule.
 */
interface AiRepository {
    suspend fun createConversation(title: String? = null): AiConversation
    suspend fun listConversations(): List<AiConversation>
    suspend fun getConversation(conversationId: String): AiConversation
    suspend fun renameConversation(conversationId: String, title: String)
    suspend fun deleteConversation(conversationId: String)
    suspend fun listMessages(conversationId: String): List<AiMessage>

    /**
     * Streams the model's reply as it's generated. The flow emits zero or
     * more [AiTurnEvent.Delta] frames followed by exactly one terminal
     * [AiTurnEvent.Completed] or [AiTurnEvent.Failed]. Collecting it is
     * what actually sends the request (cold flow) — cancelling collection
     * aborts the underlying HTTP call.
     */
    fun sendMessage(conversationId: String, content: String): Flow<AiTurnEvent>
}
