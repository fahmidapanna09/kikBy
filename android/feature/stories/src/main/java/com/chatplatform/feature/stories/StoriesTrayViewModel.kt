package com.chatplatform.feature.stories

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.stories.Story
import com.chatplatform.domain.stories.StoryFeedEntry
import com.chatplatform.domain.stories.StoriesRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class StoriesTrayState(
    val myStories: List<Story> = emptyList(),
    val entries: List<StoryFeedEntry> = emptyList(),
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
)

/** Drives the horizontal story tray at the top of the app — "My status"
 *  first, then everyone else's, unseen-first (already the order the
 *  backend returns, see stories.groupFeed). */
class StoriesTrayViewModel(private val repository: StoriesRepository) : ViewModel() {

    private val _state = MutableStateFlow(StoriesTrayState())
    val state: StateFlow<StoriesTrayState> = _state.asStateFlow()

    init {
        refresh()
        viewModelScope.launch {
            repository.storyCreatedEvents.collect { refresh() } // live: someone just posted
        }
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = it.myStories.isEmpty() && it.entries.isEmpty(), errorMessage = null) }
            try {
                val mine = repository.myActiveStories()
                val feed = repository.feed()
                _state.update { it.copy(isLoading = false, myStories = mine, entries = feed) }
            } catch (e: Exception) {
                _state.update { it.copy(isLoading = false, errorMessage = "Couldn't load stories.") }
            }
        }
    }
}
