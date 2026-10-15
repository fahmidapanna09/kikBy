package com.chatplatform.feature.channels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.channels.Channel
import com.chatplatform.domain.channels.ChannelsRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ChannelsListState(
    val mine: List<Channel> = emptyList(),
    val searchQuery: String = "",
    val searchResults: List<Channel> = emptyList(),
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
)

class ChannelsListViewModel(private val repository: ChannelsRepository) : ViewModel() {

    private val _state = MutableStateFlow(ChannelsListState())
    val state: StateFlow<ChannelsListState> = _state.asStateFlow()
    private var searchJob: Job? = null

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = it.mine.isEmpty(), errorMessage = null) }
            try {
                val mine = repository.mine()
                _state.update { it.copy(isLoading = false, mine = mine) }
            } catch (e: Exception) {
                _state.update { it.copy(isLoading = false, errorMessage = "Couldn't load your channels.") }
            }
        }
    }

    fun onSearchQueryChanged(value: String) {
        _state.update { it.copy(searchQuery = value) }
        searchJob?.cancel()
        if (value.isBlank()) {
            _state.update { it.copy(searchResults = emptyList()) }
            return
        }
        searchJob = viewModelScope.launch {
            delay(300)
            try {
                val results = repository.search(value)
                _state.update { it.copy(searchResults = results) }
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Search failed.") }
            }
        }
    }

    fun follow(channelId: String) {
        viewModelScope.launch {
            try {
                repository.follow(channelId)
                refresh()
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't follow: ${e.message}") }
            }
        }
    }

    fun followByInvite(code: String, onFollowed: (channelId: String) -> Unit) {
        viewModelScope.launch {
            try {
                val id = repository.followByInvite(code)
                refresh()
                onFollowed(id)
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't follow: ${e.message}") }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
