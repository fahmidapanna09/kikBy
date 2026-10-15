package com.chatplatform.feature.chat

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.chat.ChatMessage
import com.chatplatform.domain.chat.ChatRepository
import com.chatplatform.domain.chat.ConnectionState
import com.chatplatform.domain.chat.ReactionSummary
import com.chatplatform.domain.contacts.ContactNameProvider
import com.chatplatform.domain.media.MediaKind
import com.chatplatform.domain.media.MediaRepository
import com.chatplatform.domain.media.PendingUpload
import com.chatplatform.domain.media.UploadResult
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File
import java.io.InputStream
import java.util.UUID

data class ChatScreenState(
    val messages: List<ChatMessage> = emptyList(),
    val draft: String = "",
    val connectionState: ConnectionState = ConnectionState.CONNECTING,
    /** clientTmpId -> 0..1 progress, present only while an attachment for
     *  that (not-yet-sent) message is actively uploading. */
    val uploadProgress: Map<String, Float> = emptyMap(),
    val isRecordingVoice: Boolean = false,
    val errorMessage: String? = null,
    /** Keyed by message *server* id — see ChatRepository.observeReactions(). */
    val reactionsByMessageId: Map<String, List<ReactionSummary>> = emptyMap(),
)

/** One row in the forward-to picker: where it goes, and what to call it. */
data class ForwardTarget(val conversationId: String, val label: String)

class ChatViewModel(
    private val repository: ChatRepository,
    private val mediaRepository: MediaRepository,
    private val contactNames: ContactNameProvider,
    private val conversationId: String,
) : ViewModel() {

    private data class ExtraState(
        val uploadProgress: Map<String, Float> = emptyMap(),
        val isRecordingVoice: Boolean = false,
        val errorMessage: String? = null,
    )

    private val draft = MutableStateFlow("")
    private val extraState = MutableStateFlow(ExtraState())

    val uiState: StateFlow<ChatScreenState> = combine(
        repository.observeMessages(conversationId),
        draft,
        repository.connectionState,
        extraState,
        repository.observeReactions(),
    ) { messages, draftText, connState, extra, reactions ->
        ChatScreenState(
            messages = messages, draft = draftText, connectionState = connState,
            uploadProgress = extra.uploadProgress, isRecordingVoice = extra.isRecordingVoice, errorMessage = extra.errorMessage,
            reactionsByMessageId = reactions,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), ChatScreenState())

    init {
        viewModelScope.launch { repository.loadOlderMessages(conversationId) }
    }

    fun onDraftChanged(value: String) {
        draft.value = value
        viewModelScope.launch { repository.sendTyping(conversationId, isTyping = value.isNotEmpty()) }
    }

    fun sendMessage() {
        val text = draft.value.trim()
        if (text.isEmpty()) return
        draft.value = ""
        viewModelScope.launch {
            repository.sendMessage(conversationId, clientTmpId = UUID.randomUUID().toString(), type = "text", body = text)
            repository.sendTyping(conversationId, isTyping = false)
        }
    }

    /** Sends a copy of `message` into `targetConversationId`, tagged with
     *  forwardedFromMessageId so the recipient's client can show a
     *  "Forwarded" label. Requires message.serverId — a still-PENDING
     *  message (not yet acked) can't be forwarded yet. */
    fun forwardMessage(message: ChatMessage, targetConversationId: String) {
        val originalServerId = message.serverId ?: return
        viewModelScope.launch {
            repository.sendMessage(
                conversationId = targetConversationId,
                clientTmpId = UUID.randomUUID().toString(),
                type = message.type,
                body = message.body,
                attachmentMediaIds = message.attachmentIds,
                forwardedFromMessageId = originalServerId,
            )
        }
    }

    fun react(message: ChatMessage, emoji: String) {
        val serverId = message.serverId ?: return // can't react to a message that hasn't been acked yet
        viewModelScope.launch { repository.setReaction(serverId, emoji) }
    }

    fun removeReaction(message: ChatMessage) {
        val serverId = message.serverId ?: return
        viewModelScope.launch { repository.removeReaction(serverId) }
    }

    /** For the forward-target picker — every conversation except the one
     *  this screen is already showing, labeled with a group's name or the
     *  other person's contact name (never a raw id unless lookup failed). */
    fun forwardTargets() = repository.observeConversations().map { list ->
        list.filterNot { it.id == conversationId }.map { convo ->
            val label = when {
                convo.type == "group" -> convo.name?.takeIf { it.isNotBlank() } ?: "Group"
                convo.peerUserId != null -> contactNames.labelFor(convo.peerUserId!!)
                else -> "Chat"
            }
            ForwardTarget(convo.id, label)
        }
    }

    fun loadOlderMessages() {
        viewModelScope.launch { repository.loadOlderMessages(conversationId) }
    }

    fun markRead(messageServerId: String) {
        viewModelScope.launch { repository.markRead(conversationId, messageServerId) }
    }

    /** Presigned URLs expire, so this is called fresh right before display
     *  (e.g. from a LaunchedEffect keyed on the message), never cached
     *  long-term by the UI layer. */
    suspend fun downloadUrlFor(mediaId: String): String = mediaRepository.getDownloadUrl(mediaId)

    /**
     * Uploads an image/video/document and, on success, sends a message
     * carrying it as an attachment. The caller (Composable, which has
     * ContentResolver access) resolves the Uri into mimeType/sizeBytes and
     * an openStream lambda — this ViewModel stays free of android.content.*
     * dependencies, consistent with the rest of the module.
     */
    fun sendAttachment(kind: MediaKind, mimeType: String, fileName: String?, sizeBytes: Long, openStream: () -> InputStream) {
        val trackingId = UUID.randomUUID().toString()
        viewModelScope.launch {
            setProgress(trackingId, 0f)
            val result = mediaRepository.upload(
                PendingUpload(kind = kind, mimeType = mimeType, fileName = fileName, sizeBytes = sizeBytes, openStream = openStream),
                onProgress = { sent, total -> if (total > 0) setProgress(trackingId, sent.toFloat() / total.toFloat()) },
            )
            clearProgress(trackingId)

            when (result) {
                is UploadResult.Success -> {
                    repository.sendMessage(
                        conversationId,
                        clientTmpId = UUID.randomUUID().toString(),
                        type = kind.name.lowercase(),
                        body = null,
                        attachmentMediaIds = listOf(result.mediaId),
                    )
                }
                is UploadResult.Rejected -> setError("File rejected: ${result.reason}")
                is UploadResult.Failed -> setError(result.message)
            }
        }
    }

    /** Called with the output of VoiceRecorder.stop(). */
    fun sendVoiceNote(file: File, mimeType: String, durationMs: Long) {
        sendAttachment(
            kind = MediaKind.VOICE,
            mimeType = mimeType,
            fileName = file.name,
            sizeBytes = file.length(),
            openStream = { file.inputStream() },
        )
    }

    fun setRecordingState(isRecording: Boolean) {
        extraState.update { it.copy(isRecordingVoice = isRecording) }
    }

    fun dismissError() {
        extraState.update { it.copy(errorMessage = null) }
    }

    private fun setProgress(id: String, value: Float) {
        extraState.update { it.copy(uploadProgress = it.uploadProgress + (id to value)) }
    }

    private fun clearProgress(id: String) {
        extraState.update { it.copy(uploadProgress = it.uploadProgress - id) }
    }

    private fun setError(message: String) {
        extraState.update { it.copy(errorMessage = message) }
    }
}


