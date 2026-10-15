package com.chatplatform.feature.communities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.communities.Community
import com.chatplatform.domain.communities.CommunitiesRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CommunitiesListState(
    val mine: List<Community> = emptyList(),
    val searchQuery: String = "",
    val searchResults: List<Community> = emptyList(),
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
)

/** Drives a "Communities" tab: your own communities, plus a search box for
 *  discoverable ones you haven't joined. */
class CommunitiesListViewModel(private val repository: CommunitiesRepository) : ViewModel() {

    private val _state = MutableStateFlow(CommunitiesListState())
    val state: StateFlow<CommunitiesListState> = _state.asStateFlow()
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
                _state.update { it.copy(isLoading = false, errorMessage = "Couldn't load your communities.") }
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

    fun joinDiscoverable(communityId: String) {
        viewModelScope.launch {
            try {
                repository.joinDiscoverable(communityId)
                refresh()
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't join: ${e.message}") }
            }
        }
    }

    fun joinByInvite(inviteCode: String, onJoined: (communityId: String) -> Unit) {
        viewModelScope.launch {
            try {
                val joined = repository.joinByInvite(inviteCode)
                refresh()
                onJoined(joined.communityId)
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't join: ${e.message}") }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
