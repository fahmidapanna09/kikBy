package com.chatplatform.data.chat

import com.chatplatform.core.data.db.ConversationDao
import com.chatplatform.core.data.db.ConversationEntity
import com.chatplatform.core.data.db.MessageDao
import com.chatplatform.core.data.db.MessageEntity
import com.chatplatform.core.data.db.MessageStatus
import com.chatplatform.core.network.ChatApi
import com.chatplatform.core.network.ChatWebSocketClient
import com.chatplatform.core.network.MessageDto
import com.chatplatform.core.network.ReactionBody
import com.chatplatform.core.network.SendMessageBody
import com.chatplatform.core.network.StartConversationBody
import com.chatplatform.core.network.WsConnectionState
import com.chatplatform.core.network.WsEnvelope
import com.chatplatform.domain.chat.ChatMessage
import com.chatplatform.domain.chat.ChatRepository
import com.chatplatform.domain.chat.ConnectionState
import com.chatplatform.domain.chat.Conversation
import com.chatplatform.domain.chat.MessageDeliveryStatus
import com.chatplatform.domain.chat.ReactionSummary
import com.google.gson.Gson
import com.google.gson.JsonObject
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant

class ChatRepositoryImpl(
    private val api: ChatApi,
    private val ws: ChatWebSocketClient,
    private val messageDao: MessageDao,
    private val conversationDao: ConversationDao,
    private val currentUserId: suspend () -> String?,
    private val scope: CoroutineScope,
) : ChatRepository {

    private val gson = Gson()

    // Keyed by message *server* id. Not persisted to Room (see the
    // interface doc on observeReactions()) — lives only for this repository
    // instance's lifetime, repopulated from live WS events as they arrive.
    private val reactionsState = MutableStateFlow<Map<String, List<ReactionSummary>>>(emptyMap())

    override fun observeConversations(): Flow<List<Conversation>> =
        conversationDao.observeAll().map { list ->
            list.map {
                Conversation(
                    id = it.id, type = it.type, peerUserId = it.peerUserId, name = it.name,
                    lastMessage = null, // last-message body intentionally not denormalized onto ConversationEntity in Phase 2; join in a real impl
                    unreadCount = it.unreadCount,
                )
            }
        }

    override fun observeMessages(conversationId: String): Flow<List<ChatMessage>> =
        messageDao.observeMessages(conversationId).map { list -> list.map { it.toDomain() } }

    override val connectionState: Flow<ConnectionState> = ws.state.map {
        when (it) {
            WsConnectionState.CONNECTING -> ConnectionState.CONNECTING
            WsConnectionState.CONNECTED -> ConnectionState.CONNECTED
            WsConnectionState.DISCONNECTED -> ConnectionState.DISCONNECTED
        }
    }

    override fun observeReactions(): Flow<Map<String, List<ReactionSummary>>> = reactionsState.asStateFlow()

    override suspend fun startConversation(peerUserId: String): Conversation {
        val dto = api.startConversation(StartConversationBody(peerUserId))
        conversationDao.upsert(dto.toEntity())
        return dto.toDomain()
    }

    override suspend fun refreshConversations() {
        val res = api.listConversations()
        val fresh = res.conversations.map { it.toEntity() }
        conversationDao.upsertAll(fresh)
        if (fresh.isEmpty()) conversationDao.deleteAll() else conversationDao.deleteNotIn(fresh.map { it.id })
    }

    override suspend fun loadOlderMessages(conversationId: String) {
        // A real implementation tracks the oldest locally-known seq per
        // conversation to pass as `cursor`; omitted here for brevity —
        // wire it from messageDao's min(seq) for conversationId.
        val page = api.listMessages(conversationId, cursor = null)
        page.messages?.forEach { dto -> messageDao.upsert(dto.toEntity()) }
    }

    override suspend fun sendMessage(
        conversationId: String,
        clientTmpId: String,
        type: String,
        body: String?,
        replyToId: String?,
        attachmentMediaIds: List<String>,
        forwardedFromMessageId: String?,
    ) {
        val senderId = currentUserId() ?: return
        val entity = MessageEntity(
            clientTmpId = clientTmpId,
            serverId = null,
            conversationId = conversationId,
            senderId = senderId,
            type = type,
            body = body,
            replyToId = replyToId,
            seq = null,
            createdAtEpochMs = Instant.now().toEpochMilli(),
            status = MessageStatus.PENDING,
            attachmentIdsCsv = attachmentMediaIds.toCsv(),
            forwardedFromMessageId = forwardedFromMessageId,
        )
        messageDao.upsert(entity) // optimistic UI: shows immediately, regardless of network state
        attemptSend(entity)
    }

    private suspend fun attemptSend(entity: MessageEntity) {
        val payload = JsonObject().apply {
            addProperty("conversation_id", entity.conversationId)
            addProperty("client_tmp_id", entity.clientTmpId)
            addProperty("type", entity.type)
            entity.body?.let { addProperty("body", it) }
            entity.replyToId?.let { addProperty("reply_to_id", it) }
            entity.forwardedFromMessageId?.let { addProperty("forwarded_from_message_id", it) }
            if (entity.attachmentIds.isNotEmpty()) {
                add("attachment_media_ids", gson.toJsonTree(entity.attachmentIds))
            }
        }
        val envelope = WsEnvelope(
            event_id = java.util.UUID.randomUUID().toString(),
            type = "message.send",
            ts = Instant.now().toString(),
            conversation_id = entity.conversationId,
            payload = payload,
        )

        val sentOverWs = ws.send(envelope)
        if (sentOverWs) return // ack will arrive async via the incoming flow (see connectAndStartSyncing)

        // WS not connected right now — fall back to REST so the message
        // still goes out instead of waiting for reconnect.
        try {
            val dto = api.sendMessageRest(
                entity.conversationId,
                SendMessageBody(
                    entity.clientTmpId, entity.type, entity.body, entity.replyToId,
                    entity.attachmentIds.ifEmpty { null }, entity.forwardedFromMessageId,
                ),
            )
            messageDao.applyAck(entity.clientTmpId, dto.id, dto.seq)
        } catch (e: Exception) {
            // Leave as PENDING (not FAILED) on a network error — the outbox
            // flush on next reconnect will retry it automatically. Only
            // mark FAILED for errors that retrying won't fix (see flushOutbox).
        }
    }

    override suspend fun markDelivered(conversationId: String, messageServerId: String) {
        val payload = JsonObject().apply { addProperty("message_id", messageServerId) }
        ws.send(WsEnvelope(java.util.UUID.randomUUID().toString(), "message.delivered", Instant.now().toString(), conversation_id = conversationId, payload = payload))
    }

    override suspend fun markRead(conversationId: String, messageServerId: String) {
        val payload = JsonObject().apply { addProperty("message_id", messageServerId) }
        ws.send(WsEnvelope(java.util.UUID.randomUUID().toString(), "message.read", Instant.now().toString(), conversation_id = conversationId, payload = payload))
    }

    override suspend fun sendTyping(conversationId: String, isTyping: Boolean) {
        val type = if (isTyping) "typing.start" else "typing.stop"
        ws.send(WsEnvelope(java.util.UUID.randomUUID().toString(), type, Instant.now().toString(), conversation_id = conversationId, payload = null))
    }

    override suspend fun setReaction(messageServerId: String, emoji: String) {
        // Sent over REST rather than WS: it's not latency-critical the way a
        // message send is, and keeping it request/response makes the calling
        // code simpler (no separate ack event to correlate). The
        // authoritative reaction list still arrives for everyone — including
        // this device — via the message.reaction WS broadcast.
        try {
            api.setReaction(messageServerId, ReactionBody(emoji))
        } catch (e: Exception) {
            // Best-effort for Phase 4: a failed reaction just doesn't show up;
            // no local optimistic reaction state or retry queue yet.
        }
    }

    override suspend fun removeReaction(messageServerId: String) {
        try {
            api.removeReaction(messageServerId)
        } catch (e: Exception) {
            // See setReaction's note above.
        }
    }

    override suspend fun searchMessages(query: String, conversationId: String?): List<ChatMessage> {
        val res = api.searchMessages(query, conversationId)
        return (res.messages ?: emptyList()).map { it.toEntity().toDomain() }
    }

    override fun connectAndStartSyncing() {
        scope.launch {
            ws.incoming.collect { env -> handleIncoming(env) }
        }
        scope.launch {
            ws.state.collect { state ->
                if (state == WsConnectionState.CONNECTED) flushOutbox()
            }
        }
        ws.connect()
    }

    override fun disconnect() = ws.disconnect()

    private suspend fun flushOutbox() {
        messageDao.getOutbox().forEach { attemptSend(it) }
    }

    private suspend fun handleIncoming(env: WsEnvelope) {
        when (env.type) {
            "message.ack" -> {
                val obj = env.payload?.asJsonObject ?: return
                val clientTmpId = obj.get("client_tmp_id")?.asString ?: return
                val serverId = obj.get("server_id")?.asString ?: return
                val seq = obj.get("seq")?.asLong ?: return
                messageDao.applyAck(clientTmpId, serverId, seq)
            }
            "message.new" -> {
                val msg = gson.fromJson(env.payload, MessageDto::class.java)
                messageDao.upsert(msg.toEntity())
                // Immediately ack delivery back to the sender — this is the
                // recipient's device confirming receipt.
                val conversationId = env.conversation_id ?: msg.conversation_id
                markDelivered(conversationId, msg.id)
            }
            "message.delivered" -> {
                val obj = env.payload?.asJsonObject ?: return
                val messageId = obj.get("message_id")?.asString ?: return
                messageDao.getByServerId(messageId)?.let {
                    messageDao.updateStatus(it.clientTmpId, MessageStatus.DELIVERED)
                }
            }
            "message.read" -> {
                val obj = env.payload?.asJsonObject ?: return
                val messageId = obj.get("message_id")?.asString ?: return
                messageDao.getByServerId(messageId)?.let {
                    messageDao.updateStatus(it.clientTmpId, MessageStatus.READ)
                }
            }
            "message.reaction" -> {
                val obj = env.payload?.asJsonObject ?: return
                val messageId = obj.get("message_id")?.asString ?: return
                val reactionsArray = obj.getAsJsonArray("reactions") ?: return
                val summaries = reactionsArray.map { el ->
                    val o = el.asJsonObject
                    val emoji = o.get("emoji").asString
                    val users = o.getAsJsonArray("user_ids")?.map { it.asString } ?: emptyList()
                    ReactionSummary(emoji, users)
                }
                reactionsState.update { it + (messageId to summaries) }
            }
            "group.updated", "group.member_added", "group.member_removed" -> {
                // A group was created/renamed, or someone was added/removed
                // (possibly this user). Re-pull the conversation list so a
                // newly-added group appears, a rename shows up, and a group
                // this user just left disappears. Best-effort: a failed
                // refresh just means the next manual/periodic one catches up.
                runCatching { refreshConversations() }
            }
            // typing.start/stop, presence.update: surface via a separate
            // ephemeral flow in a full implementation (not persisted to Room,
            // since they're not durable state) — omitted here for brevity.
        }
    }
}

private fun MessageEntity.toDomain() = ChatMessage(
    clientTmpId = clientTmpId, serverId = serverId, conversationId = conversationId, senderId = senderId,
    type = type, body = body, replyToId = replyToId, seq = seq, createdAtEpochMs = createdAtEpochMs,
    status = when (status) {
        MessageStatus.PENDING -> MessageDeliveryStatus.PENDING
        MessageStatus.SENT -> MessageDeliveryStatus.SENT
        MessageStatus.DELIVERED -> MessageDeliveryStatus.DELIVERED
        MessageStatus.READ -> MessageDeliveryStatus.READ
        MessageStatus.FAILED -> MessageDeliveryStatus.FAILED
    },
    attachmentIds = attachmentIds,
    forwardedFromMessageId = forwardedFromMessageId,
)

private fun MessageDto.toEntity() = MessageEntity(
    clientTmpId = client_tmp_id, serverId = id, conversationId = conversation_id, senderId = sender_id,
    type = type, body = body, replyToId = reply_to_id, seq = seq,
    createdAtEpochMs = runCatching { Instant.parse(created_at).toEpochMilli() }.getOrDefault(Instant.now().toEpochMilli()),
    status = MessageStatus.SENT,
    attachmentIdsCsv = (attachment_ids ?: emptyList()).toCsv(),
    forwardedFromMessageId = forwarded_from_message_id,
)

private fun com.chatplatform.core.network.ConversationDto.toEntity() = ConversationEntity(
    id = id, type = type, peerUserId = peer_user_id, name = name,
    lastMessageClientTmpId = last_message?.client_tmp_id, lastMessageAtEpochMs = null, unreadCount = unread_count,
)

private fun com.chatplatform.core.network.ConversationDto.toDomain() = Conversation(
    id = id, type = type, peerUserId = peer_user_id, name = name, lastMessage = null, unreadCount = unread_count,
)
