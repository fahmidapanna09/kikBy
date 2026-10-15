package com.chatplatform.domain.channels

enum class PostType { TEXT, IMAGE, VIDEO, AUDIO, STICKER, POLL, LINK }

data class PollOption(val id: String, val text: String, val votes: Long, val votedByMe: Boolean)
data class PostReaction(val emoji: String, val count: Long)

data class ChannelPost(
    val id: String,
    val channelId: String,
    val authorId: String,
    val type: PostType,
    val body: String?,
    val mediaId: String?,
    val linkUrl: String?,
    val createdAtEpochMs: Long,
    val editedAtEpochMs: Long?,
    val viewCount: Long,
    val reactions: List<PostReaction>,
    val pollOptions: List<PollOption>,
)

data class Channel(
    val id: String,
    val name: String,
    val description: String?,
    val category: String?,
    val isPublic: Boolean,
    val followerCount: Long,
    val isFollowing: Boolean,
    val isAdmin: Boolean,
    val inviteCode: String? = null, // only present when isAdmin
)

data class ChannelAdmin(val userId: String, val isOwner: Boolean)

class ChannelActionException(message: String) : Exception(message)

interface ChannelsRepository {
    suspend fun create(name: String, description: String?, category: String?, isPublic: Boolean): Channel
    suspend fun search(query: String, category: String? = null): List<Channel>
    suspend fun mine(): List<Channel>
    suspend fun get(channelId: String): Channel
    suspend fun update(channelId: String, name: String?, description: String?, isPublic: Boolean?): Channel
    suspend fun delete(channelId: String)

    suspend fun follow(channelId: String)
    suspend fun followByInvite(inviteCode: String): String // returns the channel id
    suspend fun unfollow(channelId: String)
    suspend fun setMuted(channelId: String, muted: Boolean)

    suspend fun listAdmins(channelId: String): List<ChannelAdmin>
    suspend fun addAdmin(channelId: String, userId: String)
    suspend fun removeAdmin(channelId: String, userId: String)

    suspend fun generateInviteCode(channelId: String): String
    suspend fun revokeInviteCode(channelId: String)
    suspend fun report(channelId: String, reason: String)

    suspend fun createTextPost(channelId: String, body: String): ChannelPost
    suspend fun createMediaPost(channelId: String, type: PostType, mediaId: String, caption: String?): ChannelPost
    suspend fun createLinkPost(channelId: String, url: String, caption: String?): ChannelPost
    suspend fun createPoll(channelId: String, question: String, options: List<String>): ChannelPost
    suspend fun editPost(postId: String, body: String): ChannelPost
    suspend fun deletePost(postId: String)
    suspend fun listPosts(channelId: String, cursor: String? = null): Pair<List<ChannelPost>, String?>

    suspend fun markViewed(postId: String)
    suspend fun react(postId: String, emoji: String)
    suspend fun removeReaction(postId: String)
    suspend fun vote(postId: String, optionId: String)

    /** Live push: (channelId, postId) whenever a "channel.post" WS event
     *  arrives for a channel you follow. */
    val channelPostEvents: kotlinx.coroutines.flow.Flow<Pair<String, String>>
}
