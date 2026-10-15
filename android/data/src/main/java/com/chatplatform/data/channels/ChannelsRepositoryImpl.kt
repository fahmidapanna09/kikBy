package com.chatplatform.data.channels

import com.chatplatform.core.network.ChannelPostDto
import com.chatplatform.core.network.ChannelSummaryDto
import com.chatplatform.core.network.ChannelsApi
import com.chatplatform.core.network.ChatWebSocketClient
import com.chatplatform.core.network.CreateChannelBody
import com.chatplatform.core.network.CreatePostBody
import com.chatplatform.core.network.EditPostBody
import com.chatplatform.core.network.SetMutedBody
import com.chatplatform.core.network.StoryReactBody
import com.chatplatform.core.network.UpdateChannelBody
import com.chatplatform.core.network.VoteBody
import com.chatplatform.core.network.ChannelReportBody
import com.chatplatform.domain.channels.Channel
import com.chatplatform.domain.channels.ChannelActionException
import com.chatplatform.domain.channels.ChannelAdmin
import com.chatplatform.domain.channels.ChannelPost
import com.chatplatform.domain.channels.ChannelsRepository
import com.chatplatform.domain.channels.PollOption
import com.chatplatform.domain.channels.PostReaction
import com.chatplatform.domain.channels.PostType
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.mapNotNull
import retrofit2.HttpException
import java.io.IOException
import java.time.Instant

/** [ws] should be the SAME ChatWebSocketClient instance Chat/StoriesRepositoryImpl
 *  use — channels just also listen for "channel.post" on the shared connection,
 *  same reasoning as StoriesRepositoryImpl (see its own doc comment). */
class ChannelsRepositoryImpl(
    private val api: ChannelsApi,
    private val ws: ChatWebSocketClient,
) : ChannelsRepository {

    override val channelPostEvents: Flow<Pair<String, String>> = ws.incoming.mapNotNull { env ->
        if (env.type != "channel.post") return@mapNotNull null
        val obj = env.payload?.asJsonObject ?: return@mapNotNull null
        val channelId = obj.get("channel_id")?.asString ?: return@mapNotNull null
        val postId = obj.get("post_id")?.asString ?: return@mapNotNull null
        channelId to postId
    }

    override suspend fun create(name: String, description: String?, category: String?, isPublic: Boolean): Channel =
        call { api.create(CreateChannelBody(name, description, category = category, is_public = isPublic)).toDomain() }

    override suspend fun search(query: String, category: String?): List<Channel> =
        call { api.search(query, category).channels?.map { it.toDomain() } ?: emptyList() }

    override suspend fun mine(): List<Channel> = call { api.mine().channels?.map { it.toDomain() } ?: emptyList() }
    override suspend fun get(channelId: String): Channel = call { api.get(channelId).toDomain() }

    override suspend fun update(channelId: String, name: String?, description: String?, isPublic: Boolean?): Channel =
        call { api.update(channelId, UpdateChannelBody(name, description, is_public = isPublic)).toDomain() }

    override suspend fun delete(channelId: String) = call { api.delete(channelId) }

    override suspend fun follow(channelId: String) = call { api.follow(channelId) }
    override suspend fun followByInvite(inviteCode: String): String = call { api.followByInvite(inviteCode).channel_id }
    override suspend fun unfollow(channelId: String) = call { api.unfollow(channelId) }
    override suspend fun setMuted(channelId: String, muted: Boolean) = call { api.setMuted(channelId, SetMutedBody(muted)) }

    override suspend fun listAdmins(channelId: String): List<ChannelAdmin> =
        call { api.listAdmins(channelId).admins?.map { ChannelAdmin(it.user_id, it.role == "owner") } ?: emptyList() }

    override suspend fun addAdmin(channelId: String, userId: String) = call { api.addAdmin(channelId, userId) }
    override suspend fun removeAdmin(channelId: String, userId: String) = call { api.removeAdmin(channelId, userId) }

    override suspend fun generateInviteCode(channelId: String): String = call { api.generateInviteLink(channelId).invite_code }
    override suspend fun revokeInviteCode(channelId: String) = call { api.revokeInviteLink(channelId) }
    override suspend fun report(channelId: String, reason: String) = call { api.report(channelId, ChannelReportBody(reason)) }

    override suspend fun createTextPost(channelId: String, body: String): ChannelPost =
        call { api.createPost(channelId, CreatePostBody(type = "text", body = body)).toDomain() }

    override suspend fun createMediaPost(channelId: String, type: PostType, mediaId: String, caption: String?): ChannelPost = call {
        api.createPost(channelId, CreatePostBody(type = type.name.lowercase(), body = caption, media_id = mediaId)).toDomain()
    }

    override suspend fun createLinkPost(channelId: String, url: String, caption: String?): ChannelPost = call {
        api.createPost(channelId, CreatePostBody(type = "link", body = caption, link_url = url)).toDomain()
    }

    override suspend fun createPoll(channelId: String, question: String, options: List<String>): ChannelPost = call {
        api.createPost(channelId, CreatePostBody(type = "poll", body = question, poll_options = options)).toDomain()
    }

    override suspend fun editPost(postId: String, body: String): ChannelPost = call { api.editPost(postId, EditPostBody(body)).toDomain() }
    override suspend fun deletePost(postId: String) = call { api.deletePost(postId) }

    override suspend fun listPosts(channelId: String, cursor: String?): Pair<List<ChannelPost>, String?> = call {
        val res = api.listPosts(channelId, cursor)
        (res.posts ?: emptyList()).map { it.toDomain() } to res.next_cursor
    }

    override suspend fun markViewed(postId: String) = call { api.viewPost(postId) }
    override suspend fun react(postId: String, emoji: String) = call { api.react(postId, StoryReactBody(emoji)) }
    override suspend fun removeReaction(postId: String) = call { api.unreact(postId) }
    override suspend fun vote(postId: String, optionId: String) = call { api.vote(postId, VoteBody(optionId)) }

    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw ChannelActionException(
            when (e.code()) {
                403 -> "Only a channel admin can do that."
                404 -> "That channel, post, or invite link wasn't found."
                429 -> "You're posting too fast — try again later."
                else -> "Something went wrong (${e.code()})."
            }
        )
    } catch (e: IOException) {
        throw ChannelActionException("Couldn't reach the server. Check your connection.")
    }
}

private fun epochMs(iso: String?): Long? = iso?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }

private fun postTypeOf(s: String) = when (s) {
    "image" -> PostType.IMAGE
    "video" -> PostType.VIDEO
    "audio" -> PostType.AUDIO
    "sticker" -> PostType.STICKER
    "poll" -> PostType.POLL
    "link" -> PostType.LINK
    else -> PostType.TEXT
}

private fun ChannelSummaryDto.toDomain() = Channel(
    id = id, name = name, description = description, category = category, isPublic = is_public,
    followerCount = follower_count, isFollowing = is_following, isAdmin = is_admin, inviteCode = invite_code,
)

private fun ChannelPostDto.toDomain() = ChannelPost(
    id = id, channelId = channel_id, authorId = author_id, type = postTypeOf(type), body = body, mediaId = media_id,
    linkUrl = link_url, createdAtEpochMs = epochMs(created_at) ?: 0L, editedAtEpochMs = epochMs(edited_at),
    viewCount = view_count,
    reactions = (reactions ?: emptyList()).map { PostReaction(it.emoji, it.count) },
    pollOptions = (poll_options ?: emptyList()).map { PollOption(it.id, it.text, it.votes, it.voted_by_me) },
)
