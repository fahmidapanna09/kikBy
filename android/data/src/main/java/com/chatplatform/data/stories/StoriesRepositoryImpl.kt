package com.chatplatform.data.stories

import com.chatplatform.core.network.ChatWebSocketClient
import com.chatplatform.core.network.CreateStoryBody
import com.chatplatform.core.network.StoriesApi
import com.chatplatform.core.network.StoryDto
import com.chatplatform.core.network.StoryFeedEntryDto
import com.chatplatform.core.network.StoryReactBody
import com.chatplatform.core.network.StoryReplyBody
import com.chatplatform.core.network.StoryReplyDto
import com.chatplatform.core.network.StoryViewerDto
import com.chatplatform.domain.stories.NewStory
import com.chatplatform.domain.stories.Story
import com.chatplatform.domain.stories.StoryActionException
import com.chatplatform.domain.stories.StoryFeedEntry
import com.chatplatform.domain.stories.StoryPrivacy
import com.chatplatform.domain.stories.StoryReply
import com.chatplatform.domain.stories.StoryType
import com.chatplatform.domain.stories.StoryViewer
import com.chatplatform.domain.stories.StoriesRepository
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.mapNotNull
import retrofit2.HttpException
import java.io.IOException
import java.time.Instant

/**
 * [ws] should be the SAME ChatWebSocketClient instance ChatRepositoryImpl
 * uses (wire one singleton at the DI layer) — stories don't open a second
 * connection, they just also listen for "story.created" on the existing one.
 */
class StoriesRepositoryImpl(
    private val api: StoriesApi,
    private val ws: ChatWebSocketClient,
) : StoriesRepository {

    override val storyCreatedEvents: Flow<Pair<String, String>> = ws.incoming.mapNotNull { env ->
        if (env.type != "story.created") return@mapNotNull null
        val obj = env.payload?.asJsonObject ?: return@mapNotNull null
        val storyId = obj.get("story_id")?.asString ?: return@mapNotNull null
        val authorId = obj.get("author_id")?.asString ?: return@mapNotNull null
        storyId to authorId
    }

    override suspend fun feed(): List<StoryFeedEntry> = call { api.feed().entries?.map { it.toDomain() } ?: emptyList() }

    override suspend fun myActiveStories(): List<Story> = call { api.mine().stories?.map { it.toDomain() } ?: emptyList() }

    override suspend fun archive(beforeEpochMs: Long?): List<Story> =
        call { api.archive(beforeEpochMs).stories?.map { it.toDomain() } ?: emptyList() }

    override suspend fun create(story: NewStory): Story = call {
        api.create(
            CreateStoryBody(
                type = story.type.name.lowercase(),
                text_content = story.textContent,
                background_color = story.backgroundColor,
                caption = story.caption,
                media_id = story.mediaId,
                privacy = story.privacy.name.lowercase(),
                audience_user_ids = story.audienceUserIds.ifEmpty { null },
                mention_user_ids = story.mentionUserIds.ifEmpty { null },
            )
        ).toDomain()
    }

    override suspend fun delete(storyId: String) = call { api.delete(storyId) }
    override suspend fun markViewed(storyId: String) = call { api.view(storyId) }
    override suspend fun viewers(storyId: String): List<StoryViewer> = call { api.viewers(storyId).viewers?.map { it.toDomain() } ?: emptyList() }
    override suspend fun react(storyId: String, emoji: String) = call { api.react(storyId, StoryReactBody(emoji)) }
    override suspend fun removeReaction(storyId: String) = call { api.unreact(storyId) }
    override suspend fun reply(storyId: String, body: String): StoryReply = call { api.reply(storyId, StoryReplyBody(body)).toDomain() }
    override suspend fun repliesToMyStory(storyId: String): List<StoryReply> = call { api.replies(storyId).replies?.map { it.toDomain() } ?: emptyList() }
    override suspend fun mute(userId: String) = call { api.mute(userId) }
    override suspend fun unmute(userId: String) = call { api.unmute(userId) }

    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw StoryActionException(
            when (e.code()) {
                403 -> "You can't do that with this story."
                404 -> "That story isn't available anymore."
                429 -> "You're doing that too much — try again later."
                else -> "Something went wrong (${e.code()})."
            }
        )
    } catch (e: IOException) {
        throw StoryActionException("Couldn't reach the server. Check your connection.")
    }
}

private fun epochMs(iso: String): Long = runCatching { Instant.parse(iso).toEpochMilli() }.getOrDefault(0L)

private fun StoryDto.toDomain() = Story(
    id = id, authorId = author_id,
    type = when (type) { "image" -> StoryType.IMAGE; "video" -> StoryType.VIDEO; else -> StoryType.TEXT },
    textContent = text_content, backgroundColor = background_color, caption = caption, mediaId = media_id,
    createdAtEpochMs = epochMs(created_at), expiresAtEpochMs = epochMs(expires_at), seen = seen,
    privacy = when (privacy) {
        "contacts_except" -> StoryPrivacy.CONTACTS_EXCEPT
        "selected" -> StoryPrivacy.SELECTED
        "contacts" -> StoryPrivacy.CONTACTS
        else -> null
    },
    viewCount = view_count, reactionCount = reaction_count, replyCount = reply_count,
)

private fun StoryFeedEntryDto.toDomain() = StoryFeedEntry(
    authorId = author_id, username = username, displayName = display_name, muted = muted, hasUnseen = has_unseen,
    stories = stories.map { it.toDomain() },
)

private fun StoryViewerDto.toDomain() = StoryViewer(
    userId = user_id, username = username, displayName = display_name, viewedAtEpochMs = epochMs(viewed_at), reaction = reaction,
)

private fun StoryReplyDto.toDomain() = StoryReply(id = id, storyId = story_id, replierId = replier_id, body = body, createdAtEpochMs = epochMs(created_at))
