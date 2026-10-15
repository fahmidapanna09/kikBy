package com.chatplatform.data.ai

import com.chatplatform.core.network.AiApi
import com.chatplatform.core.network.AiConversationDto
import com.chatplatform.core.network.AiMessageDto
import com.chatplatform.core.network.AiStreamClient
import com.chatplatform.core.network.AiStreamEvent
import com.chatplatform.core.network.CreateAiConversationBody
import com.chatplatform.core.network.UpdateAiConversationBody
import com.chatplatform.domain.ai.AiActionException
import com.chatplatform.domain.ai.AiConversation
import com.chatplatform.domain.ai.AiMessage
import com.chatplatform.domain.ai.AiRepository
import com.chatplatform.domain.ai.AiRole
import com.chatplatform.domain.ai.AiTurnEvent
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import retrofit2.HttpException
import java.io.IOException
import java.time.Instant

class AiRepositoryImpl(
    private val api: AiApi,
    private val streamClient: AiStreamClient,
) : AiRepository {

    override suspend fun createConversation(title: String?): AiConversation =
        call { api.createConversation(CreateAiConversationBody(title)).toDomain() }

    override suspend fun listConversations(): List<AiConversation> =
        call { api.listConversations().conversations?.map { it.toDomain() } ?: emptyList() }

    override suspend fun getConversation(conversationId: String): AiConversation =
        call { api.getConversation(conversationId).toDomain() }

    override suspend fun renameConversation(conversationId: String, title: String) {
        call { api.renameConversation(conversationId, UpdateAiConversationBody(title)) }
    }

    override suspend fun deleteConversation(conversationId: String) {
        call { api.deleteConversation(conversationId) }
    }

    override suspend fun listMessages(conversationId: String): List<AiMessage> =
        call { api.listMessages(conversationId).messages?.map { it.toDomain() } ?: emptyList() }

    override fun sendMessage(conversationId: String, content: String): Flow<AiTurnEvent> =
        streamClient.sendMessage(conversationId, content).map { event ->
            when (event) {
                is AiStreamEvent.Delta -> AiTurnEvent.Delta(event.text)
                is AiStreamEvent.Done -> AiTurnEvent.Completed(event.message.toDomain())
                is AiStreamEvent.StreamError -> AiTurnEvent.Failed(event.message)
            }
        }

    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw AiActionException(
            when (e.code()) {
                404 -> "That conversation wasn't found."
                429 -> "You've hit the AI message limit for now — try again later."
                else -> "Something went wrong (${e.code()})."
            }
        )
    } catch (e: IOException) {
        throw AiActionException("Couldn't reach the server. Check your connection.")
    }
}

private fun epochMs(iso: String): Long = runCatching { Instant.parse(iso).toEpochMilli() }.getOrDefault(0L)

private fun AiConversationDto.toDomain() = AiConversation(
    id = id,
    title = title,
    model = model,
    createdAtEpochMs = epochMs(created_at),
    updatedAtEpochMs = epochMs(updated_at),
)

private fun AiMessageDto.toDomain() = AiMessage(
    id = id,
    conversationId = conversation_id,
    role = if (role == "model") AiRole.MODEL else AiRole.USER,
    content = content,
    finishReason = finish_reason,
    error = error,
    createdAtEpochMs = epochMs(created_at),
)
