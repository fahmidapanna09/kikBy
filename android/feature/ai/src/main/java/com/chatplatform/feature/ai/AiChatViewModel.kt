package com.chatplatform.feature.ai

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.ai.AiMessage
import com.chatplatform.domain.ai.AiRepository
import com.chatplatform.domain.ai.AiRole
import com.chatplatform.domain.ai.AiTurnEvent
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class AiChatState(
    val title: String = "AI Chat",
    val messages: List<AiMessage> = emptyList(),
    val isLoadingHistory: Boolean = true,
    val draftInput: String = "",
    val isSending: Boolean = false,
    // Accumulated deltas for the reply currently streaming in, rendered as
    // a temporary last bubble until the turn completes and loadHistory()
    // replaces it with the canonical persisted message.
    val streamingText: String = "",
    val errorMessage: String? = null,
)

/** One per open AI conversation screen; [conversationId] is fixed for its lifetime. */
class AiChatViewModel(
    private val repository: AiRepository,
    private val conversationId: String,
) : ViewModel() {

    private val _state = MutableStateFlow(AiChatState())
    val state: StateFlow<AiChatState> = _state.asStateFlow()

    init {
        loadHistory()
    }

    private fun loadHistory() {
        viewModelScope.launch {
            _state.update { it.copy(isLoadingHistory = it.messages.isEmpty(), errorMessage = null) }
            try {
                val conv = runCatching { repository.getConversation(conversationId) }.getOrNull()
                val messages = repository.listMessages(conversationId)
                _state.update {
                    it.copy(
                        isLoadingHistory = false,
                        messages = messages,
                        title = conv?.title ?: it.title,
                    )
                }
            } catch (e: Exception) {
                _state.update { it.copy(isLoadingHistory = false, errorMessage = "Couldn't load this chat.") }
            }
        }
    }

    fun onInputChanged(value: String) {
        _state.update { it.copy(draftInput = value) }
    }

    fun send() {
        val text = _state.value.draftInput.trim()
        if (text.isEmpty() || _state.value.isSending) return

        // Optimistic bubble so the user sees what they typed immediately,
        // instead of waiting for the first streamed delta. A temp id keeps
        // it from colliding with the real message once loadHistory() runs.
        val optimisticUserMessage = AiMessage(
            id = "optimistic-${System.currentTimeMillis()}",
            conversationId = conversationId,
            role = AiRole.USER,
            content = text,
            finishReason = null,
            error = null,
            createdAtEpochMs = System.currentTimeMillis(),
        )
        _state.update {
            it.copy(
                messages = it.messages + optimisticUserMessage,
                draftInput = "",
                isSending = true,
                streamingText = "",
                errorMessage = null,
            )
        }

        viewModelScope.launch {
            repository.sendMessage(conversationId, text).collect { event ->
                when (event) {
                    is AiTurnEvent.Delta -> _state.update { it.copy(streamingText = it.streamingText + event.text) }
                    is AiTurnEvent.Completed -> {
                        _state.update { it.copy(isSending = false, streamingText = "") }
                        loadHistory() // replaces the optimistic bubble with the canonical persisted pair
                    }
                    is AiTurnEvent.Failed -> _state.update {
                        it.copy(isSending = false, streamingText = "", errorMessage = event.reason)
                    }
                }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
