package com.chatplatform.feature.ai

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.ai.AiConversation
import com.chatplatform.domain.ai.AiRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class AiConversationListState(
    val conversations: List<AiConversation> = emptyList(),
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
)

class AiConversationListViewModel(private val repository: AiRepository) : ViewModel() {

    private val _state = MutableStateFlow(AiConversationListState())
    val state: StateFlow<AiConversationListState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = it.conversations.isEmpty(), errorMessage = null) }
            try {
                val list = repository.listConversations()
                _state.update { it.copy(isLoading = false, conversations = list) }
            } catch (e: Exception) {
                _state.update { it.copy(isLoading = false, errorMessage = "Couldn't load your AI chats.") }
            }
        }
    }

    /** Creates a new conversation and reports its id back via [onCreated] so the caller can navigate into it. */
    fun createConversation(onCreated: (conversationId: String) -> Unit) {
        viewModelScope.launch {
            try {
                val conv = repository.createConversation()
                refresh()
                onCreated(conv.id)
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't start a new chat: ${e.message}") }
            }
        }
    }

    fun delete(conversationId: String) {
        viewModelScope.launch {
            try {
                repository.deleteConversation(conversationId)
                refresh()
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't delete: ${e.message}") }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
