package com.chatplatform.feature.channels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.channels.Channel
import com.chatplatform.domain.channels.ChannelActionException
import com.chatplatform.domain.channels.ChannelPost
import com.chatplatform.domain.channels.ChannelsRepository
import com.chatplatform.domain.media.MediaKind
import com.chatplatform.domain.media.MediaRepository
import com.chatplatform.domain.media.PendingUpload
import com.chatplatform.domain.media.UploadResult
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.InputStream

data class ChannelDetailState(
    val channel: Channel? = null,
    val posts: List<ChannelPost> = emptyList(),
    val nextCursor: String? = null,
    val isLoading: Boolean = true,
    val isLoadingMore: Boolean = false,
    val isPosting: Boolean = false,
    val uploadProgress: Float? = null,
    val errorMessage: String? = null,
    val deleted: Boolean = false,
)

class ChannelDetailViewModel(
    private val repository: ChannelsRepository,
    private val mediaRepository: MediaRepository,
    private val channelId: String,
    private val currentUserId: String,
) : ViewModel() {

    private val _state = MutableStateFlow(ChannelDetailState())
    val state: StateFlow<ChannelDetailState> = _state.asStateFlow()

    init {
        refresh()
        viewModelScope.launch {
            repository.channelPostEvents.collect { (chId, _) -> if (chId == channelId) refresh() }
        }
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(isLoading = it.channel == null, errorMessage = null) }
            try {
                val channel = repository.get(channelId)
                val (posts, cursor) = repository.listPosts(channelId)
                _state.update { it.copy(isLoading = false, channel = channel, posts = posts, nextCursor = cursor) }
                posts.forEach { runCatching { repository.markViewed(it.id) } }
            } catch (e: ChannelActionException) {
                _state.update { it.copy(isLoading = false, errorMessage = e.message) }
            }
        }
    }

    fun loadMore() {
        val cursor = _state.value.nextCursor ?: return
        if (_state.value.isLoadingMore) return
        viewModelScope.launch {
            _state.update { it.copy(isLoadingMore = true) }
            try {
                val (more, nextCursor) = repository.listPosts(channelId, cursor)
                _state.update { it.copy(isLoadingMore = false, posts = it.posts + more, nextCursor = nextCursor) }
                more.forEach { runCatching { repository.markViewed(it.id) } }
            } catch (e: ChannelActionException) {
                _state.update { it.copy(isLoadingMore = false, errorMessage = e.message) }
            }
        }
    }

    fun postText(body: String) = post { repository.createTextPost(channelId, body) }

    fun postPoll(question: String, options: List<String>) = post { repository.createPoll(channelId, question, options) }

    fun postMedia(kind: MediaKind, mimeType: String, sizeBytes: Long, caption: String?, openStream: () -> InputStream) {
        viewModelScope.launch {
            _state.update { it.copy(isPosting = true, uploadProgress = 0f, errorMessage = null) }
            val result = mediaRepository.upload(
                PendingUpload(kind, mimeType, fileName = null, sizeBytes = sizeBytes, openStream = openStream),
                onProgress = { sent, total -> if (total > 0) _state.update { it.copy(uploadProgress = sent.toFloat() / total.toFloat()) } },
            )
            when (result) {
                is UploadResult.Success -> post {
                    val postType = when (kind) {
                        MediaKind.IMAGE -> com.chatplatform.domain.channels.PostType.IMAGE
                        MediaKind.VIDEO -> com.chatplatform.domain.channels.PostType.VIDEO
                        MediaKind.AUDIO, MediaKind.VOICE -> com.chatplatform.domain.channels.PostType.AUDIO
                        else -> com.chatplatform.domain.channels.PostType.IMAGE
                    }
                    repository.createMediaPost(channelId, postType, result.mediaId, caption)
                }
                is UploadResult.Rejected -> _state.update { it.copy(isPosting = false, uploadProgress = null, errorMessage = "Rejected: ${result.reason}") }
                is UploadResult.Failed -> _state.update { it.copy(isPosting = false, uploadProgress = null, errorMessage = result.message) }
            }
        }
    }

    private fun post(block: suspend () -> ChannelPost) {
        viewModelScope.launch {
            _state.update { it.copy(isPosting = true, errorMessage = null) }
            try {
                block()
                refresh()
            } catch (e: ChannelActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            } finally {
                _state.update { it.copy(isPosting = false, uploadProgress = null) }
            }
        }
    }

    fun deletePost(postId: String) = act { repository.deletePost(postId) }
    fun react(postId: String, emoji: String) = act { repository.react(postId, emoji) }
    fun removeReaction(postId: String) = act { repository.removeReaction(postId) }
    fun vote(postId: String, optionId: String) = act { repository.vote(postId, optionId) }

    fun follow() = act { repository.follow(channelId) }
    fun unfollow() = act { repository.unfollow(channelId) }
    fun setMuted(muted: Boolean) = act { repository.setMuted(channelId, muted) }

    fun generateInviteCode() = act { repository.generateInviteCode(channelId) }
    fun revokeInviteCode() = act { repository.revokeInviteCode(channelId) }
    fun addAdmin(userId: String) = act { repository.addAdmin(channelId, userId) }
    fun removeAdmin(userId: String) = act { repository.removeAdmin(channelId, userId) }
    fun report(reason: String) = act { repository.report(channelId, reason) }

    fun deleteChannel() {
        viewModelScope.launch {
            try {
                repository.delete(channelId)
                _state.update { it.copy(deleted = true) }
            } catch (e: ChannelActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }

    private fun act(block: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                block()
                refresh()
            } catch (e: ChannelActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }
}
