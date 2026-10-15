package com.chatplatform.feature.chat

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.chat.ChatMessage
import com.chatplatform.domain.chat.ChatRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SearchUiState(
    val query: String = "",
    val results: List<ChatMessage> = emptyList(),
    val isSearching: Boolean = false,
    val errorMessage: String? = null,
)

/**
 * conversationId == null searches across every conversation the user is a
 * member of (server-enforced); pass a specific id to scope search to one
 * conversation (e.g. a "search in this chat" entry point from ChatScreen).
 */
class SearchViewModel(
    private val repository: ChatRepository,
    private val conversationId: String? = null,
) : ViewModel() {

    private val _uiState = MutableStateFlow(SearchUiState())
    val uiState: StateFlow<SearchUiState> = _uiState.asStateFlow()

    private var debounceJob: Job? = null

    fun onQueryChanged(value: String) {
        _uiState.update { it.copy(query = value, errorMessage = null) }
        debounceJob?.cancel()
        if (value.isBlank()) {
            _uiState.update { it.copy(results = emptyList(), isSearching = false) }
            return
        }
        debounceJob = viewModelScope.launch {
            delay(300) // avoid firing a request on every keystroke
            runSearch(value)
        }
    }

    private suspend fun runSearch(query: String) {
        _uiState.update { it.copy(isSearching = true) }
        try {
            val results = repository.searchMessages(query, conversationId)
            _uiState.update { it.copy(results = results, isSearching = false) }
        } catch (e: Exception) {
            _uiState.update { it.copy(isSearching = false, errorMessage = "Search failed: ${e.message}") }
        }
    }
}
