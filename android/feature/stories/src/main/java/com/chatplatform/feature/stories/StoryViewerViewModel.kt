package com.chatplatform.feature.stories

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.media.MediaRepository
import com.chatplatform.domain.stories.Story
import com.chatplatform.domain.stories.StoryFeedEntry
import com.chatplatform.domain.stories.StoryType
import com.chatplatform.domain.stories.StoriesRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

private val TEXT_STORY_DURATION_MS = 5_000L
private val MEDIA_STORY_DURATION_MS = 7_000L // videos ideally use their own duration; a fixed fallback keeps this simple

data class StoryViewerState(
    val entries: List<StoryFeedEntry> = emptyList(),
    val entryIndex: Int = 0,
    val storyIndex: Int = 0,
    val progress: Float = 0f, // 0..1 within the current story
    val isPaused: Boolean = false,
    val mediaUrl: String? = null,
    val replyDraft: String = "",
    val closed: Boolean = false,
    val errorMessage: String? = null,
) {
    val currentEntry: StoryFeedEntry? get() = entries.getOrNull(entryIndex)
    val currentStory: Story? get() = currentEntry?.stories?.getOrNull(storyIndex)
}

/** [isOwnStories] true when [entries] is really just the viewer's own
 *  active stories (opened from "My status") — disables reply/react (you
 *  don't react to yourself) and the caller should show a viewer-count
 *  affordance instead. */
class StoryViewerViewModel(
    private val repository: StoriesRepository,
    private val mediaRepository: MediaRepository,
    initialEntries: List<StoryFeedEntry>,
    startEntryIndex: Int,
    val isOwnStories: Boolean = false,
) : ViewModel() {

    private val _state = MutableStateFlow(StoryViewerState(entries = initialEntries, entryIndex = startEntryIndex))
    val state: StateFlow<StoryViewerState> = _state.asStateFlow()

    private var playJob: Job? = null

    init {
        enterStory()
    }

    private fun enterStory() {
        playJob?.cancel()
        val story = _state.value.currentStory
        if (story == null) {
            _state.update { it.copy(closed = true) }
            return
        }
        _state.update { it.copy(progress = 0f, mediaUrl = null, replyDraft = "") }

        viewModelScope.launch {
            if (!isOwnStories) repository.markViewed(story.id) // no-op server-side to view your own; skip the call either way

            if (story.type != StoryType.TEXT && story.mediaId != null) {
                val url = runCatching { mediaRepository.getDownloadUrl(story.mediaId) }.getOrNull()
                _state.update { it.copy(mediaUrl = url) }
            }
        }

        val duration = if (story.type == StoryType.TEXT) TEXT_STORY_DURATION_MS else MEDIA_STORY_DURATION_MS
        playJob = viewModelScope.launch {
            val stepMs = 50L
            var elapsed = 0L
            while (elapsed < duration) {
                if (!_state.value.isPaused) {
                    elapsed += stepMs
                    _state.update { it.copy(progress = (elapsed.toFloat() / duration).coerceIn(0f, 1f)) }
                }
                delay(stepMs)
            }
            next()
        }
    }

    fun next() {
        val s = _state.value
        val entry = s.currentEntry ?: return
        if (s.storyIndex + 1 < entry.stories.size) {
            _state.update { it.copy(storyIndex = it.storyIndex + 1) }
        } else if (s.entryIndex + 1 < s.entries.size) {
            _state.update { it.copy(entryIndex = it.entryIndex + 1, storyIndex = 0) }
        } else {
            _state.update { it.copy(closed = true) }
            return
        }
        enterStory()
    }

    fun previous() {
        val s = _state.value
        if (s.storyIndex > 0) {
            _state.update { it.copy(storyIndex = it.storyIndex - 1) }
        } else if (s.entryIndex > 0) {
            val prevEntry = s.entries[s.entryIndex - 1]
            _state.update { it.copy(entryIndex = it.entryIndex - 1, storyIndex = (prevEntry.stories.size - 1).coerceAtLeast(0)) }
        } else {
            return // already at the very first story — stay put rather than closing
        }
        enterStory()
    }

    fun setPaused(paused: Boolean) = _state.update { it.copy(isPaused = paused) }

    fun onReplyDraftChanged(value: String) = _state.update { it.copy(replyDraft = value) }

    fun sendReply() {
        val story = _state.value.currentStory ?: return
        val body = _state.value.replyDraft.trim()
        if (body.isEmpty()) return
        _state.update { it.copy(replyDraft = "") }
        viewModelScope.launch {
            try {
                repository.reply(story.id, body)
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Reply didn't send.") }
            }
        }
    }

    fun react(emoji: String) {
        val story = _state.value.currentStory ?: return
        viewModelScope.launch {
            try {
                repository.react(story.id, emoji)
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Reaction didn't send.") }
            }
        }
    }

    fun deleteCurrent(onDeleted: () -> Unit) {
        val story = _state.value.currentStory ?: return
        viewModelScope.launch {
            try {
                repository.delete(story.id)
                onDeleted()
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Couldn't delete that story.") }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
