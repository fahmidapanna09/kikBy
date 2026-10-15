package com.chatplatform.feature.stories

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.media.MediaKind
import com.chatplatform.domain.media.MediaRepository
import com.chatplatform.domain.media.PendingUpload
import com.chatplatform.domain.media.UploadResult
import com.chatplatform.domain.stories.NewStory
import com.chatplatform.domain.stories.StoryPrivacy
import com.chatplatform.domain.stories.StoryType
import com.chatplatform.domain.stories.StoriesRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.InputStream

/** A small fixed palette rather than a full color picker — keeps this
 *  screen simple; any "#RRGGBB" the backend accepts would work. */
val STORY_BACKGROUND_COLORS = listOf("#1F6F5C", "#7A3B8F", "#B14A32", "#2E4C8F", "#3A3A3A")

data class CreateStoryState(
    val text: String = "",
    val backgroundColor: String = STORY_BACKGROUND_COLORS.first(),
    val isPosting: Boolean = false,
    val uploadProgress: Float? = null, // non-null only while a media upload is in flight
    val errorMessage: String? = null,
    val posted: Boolean = false,
)

/**
 * Privacy is fixed at "contacts" (everyone who has you saved) in this pass —
 * the contacts_except/selected modes need an audience picker, which would
 * duplicate feature/groups's MemberPicker; see the stories README note for
 * why that's deferred rather than built here.
 */
class CreateStoryViewModel(
    private val repository: StoriesRepository,
    private val mediaRepository: MediaRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(CreateStoryState())
    val state: StateFlow<CreateStoryState> = _state.asStateFlow()

    fun onTextChanged(value: String) = _state.update { it.copy(text = value.take(700), errorMessage = null) }
    fun onBackgroundChanged(color: String) = _state.update { it.copy(backgroundColor = color) }

    fun postText() {
        val text = _state.value.text.trim()
        if (text.isEmpty()) {
            _state.update { it.copy(errorMessage = "Write something first.") }
            return
        }
        post(NewStory(type = StoryType.TEXT, textContent = text, backgroundColor = _state.value.backgroundColor, privacy = StoryPrivacy.CONTACTS))
    }

    /** Caller resolves the picked Uri the same way ChatScreen does for chat
     *  attachments — see MediaKind.IMAGE/VIDEO usage there. */
    fun postMedia(kind: MediaKind, mimeType: String, sizeBytes: Long, caption: String?, openStream: () -> InputStream) {
        if (kind != MediaKind.IMAGE && kind != MediaKind.VIDEO) {
            _state.update { it.copy(errorMessage = "Stories support photos and videos.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isPosting = true, uploadProgress = 0f, errorMessage = null) }
            val result = mediaRepository.upload(
                PendingUpload(kind = kind, mimeType = mimeType, fileName = null, sizeBytes = sizeBytes, openStream = openStream),
                onProgress = { sent, total -> if (total > 0) _state.update { it.copy(uploadProgress = sent.toFloat() / total.toFloat()) } },
            )
            when (result) {
                is UploadResult.Success -> post(
                    NewStory(
                        type = if (kind == MediaKind.IMAGE) StoryType.IMAGE else StoryType.VIDEO,
                        mediaId = result.mediaId, caption = caption?.take(700), privacy = StoryPrivacy.CONTACTS,
                    ),
                )
                is UploadResult.Rejected -> _state.update { it.copy(isPosting = false, uploadProgress = null, errorMessage = "Rejected: ${result.reason}") }
                is UploadResult.Failed -> _state.update { it.copy(isPosting = false, uploadProgress = null, errorMessage = result.message) }
            }
        }
    }

    private fun post(story: NewStory) {
        viewModelScope.launch {
            _state.update { it.copy(isPosting = true) }
            try {
                repository.create(story)
                _state.update { it.copy(isPosting = false, uploadProgress = null, posted = true) }
            } catch (e: Exception) {
                _state.update { it.copy(isPosting = false, uploadProgress = null, errorMessage = "Couldn't post: ${e.message}") }
            }
        }
    }
}
