package com.chatplatform.domain.stories

enum class StoryType { TEXT, IMAGE, VIDEO }
enum class StoryPrivacy { CONTACTS, CONTACTS_EXCEPT, SELECTED }

data class Story(
    val id: String,
    val authorId: String,
    val type: StoryType,
    val textContent: String?,
    val backgroundColor: String?,
    val caption: String?,
    val mediaId: String?,
    val createdAtEpochMs: Long,
    val expiresAtEpochMs: Long,
    val seen: Boolean,
    val privacy: StoryPrivacy? = null,       // set on your own stories only
    val viewCount: Long? = null,             // set on your own stories only
    val reactionCount: Long? = null,         // set on your own stories only
    val replyCount: Long? = null,            // set on your own stories only
)

data class StoryFeedEntry(
    val authorId: String,
    val username: String,
    val displayName: String?,
    val muted: Boolean,
    val hasUnseen: Boolean,
    val stories: List<Story>, // oldest first — the order they play in
)

data class StoryViewer(
    val userId: String,
    val username: String,
    val displayName: String?,
    val viewedAtEpochMs: Long,
    val reaction: String?,
)

data class StoryReply(val id: String, val storyId: String, val replierId: String, val body: String, val createdAtEpochMs: Long)

/** What CreateStoryScreen builds and hands to StoriesRepository.create(). */
data class NewStory(
    val type: StoryType,
    val textContent: String? = null,
    val backgroundColor: String? = null,
    val caption: String? = null,
    val mediaId: String? = null,
    val privacy: StoryPrivacy = StoryPrivacy.CONTACTS,
    val audienceUserIds: List<String> = emptyList(),
    val mentionUserIds: List<String> = emptyList(),
)

class StoryActionException(message: String) : Exception(message)

interface StoriesRepository {
    /** Grouped by author, ordered the way StoriesScreen's tray should show
     *  it (unseen authors first) — see backend's groupFeed for the exact rule. */
    suspend fun feed(): List<StoryFeedEntry>

    suspend fun myActiveStories(): List<Story>
    suspend fun archive(beforeEpochMs: Long? = null): List<Story>

    suspend fun create(story: NewStory): Story
    suspend fun delete(storyId: String)

    suspend fun markViewed(storyId: String)
    suspend fun viewers(storyId: String): List<StoryViewer>

    suspend fun react(storyId: String, emoji: String)
    suspend fun removeReaction(storyId: String)

    suspend fun reply(storyId: String, body: String): StoryReply
    suspend fun repliesToMyStory(storyId: String): List<StoryReply>

    suspend fun mute(userId: String)
    suspend fun unmute(userId: String)

    /** Live push: emits a (storyId, authorId) pair whenever a "story.created"
     *  WS event arrives, so the tray can refresh without polling. */
    val storyCreatedEvents: kotlinx.coroutines.flow.Flow<Pair<String, String>>
}
