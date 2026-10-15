package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path
import retrofit2.http.Query

data class CreateChannelBody(val name: String, val description: String? = null, val icon_media_id: String? = null, val category: String? = null, val is_public: Boolean = true)
data class UpdateChannelBody(val name: String? = null, val description: String? = null, val icon_media_id: String? = null, val category: String? = null, val is_public: Boolean? = null)
data class ChannelSummaryDto(
    val id: String, val name: String, val description: String?, val icon_media_id: String?, val category: String?,
    val is_public: Boolean, val follower_count: Long, val is_following: Boolean, val is_admin: Boolean, val invite_code: String?,
)
data class ChannelSearchResponseDto(val channels: List<ChannelSummaryDto>?)
data class ChannelInviteLinkResponseDto(val invite_code: String)
data class ChannelFollowResponseDto(val channel_id: String)
data class ChannelAdminDto(val user_id: String, val role: String, val added_at: String)
data class ChannelAdminsResponseDto(val admins: List<ChannelAdminDto>?)
data class SetMutedBody(val muted: Boolean)
data class ChannelReportBody(val reason: String)

data class CreatePostBody(
    val type: String, val body: String? = null, val media_id: String? = null, val link_url: String? = null,
    val poll_options: List<String>? = null,
)
data class EditPostBody(val body: String)
data class PollOptionDto(val id: String, val text: String, val votes: Long, val voted_by_me: Boolean)
data class PostReactionSummaryDto(val emoji: String, val count: Long)
data class ChannelPostDto(
    val id: String, val channel_id: String, val author_id: String, val seq: Long, val type: String,
    val body: String?, val media_id: String?, val link_url: String?, val created_at: String, val edited_at: String?,
    val view_count: Long, val reactions: List<PostReactionSummaryDto>?, val poll_options: List<PollOptionDto>?,
)
data class ChannelPostsResponseDto(val posts: List<ChannelPostDto>?, val next_cursor: String?)
data class VoteBody(val option_id: String)

interface ChannelsApi {
    @POST("v1/channels")
    suspend fun create(@Body body: CreateChannelBody): ChannelSummaryDto

    @GET("v1/channels/search")
    suspend fun search(@Query("q") query: String, @Query("category") category: String? = null): ChannelSearchResponseDto

    @GET("v1/channels/mine")
    suspend fun mine(): ChannelSearchResponseDto

    @GET("v1/channels/{channelId}")
    suspend fun get(@Path("channelId") channelId: String): ChannelSummaryDto

    @PATCH("v1/channels/{channelId}")
    suspend fun update(@Path("channelId") channelId: String, @Body body: UpdateChannelBody): ChannelSummaryDto

    @DELETE("v1/channels/{channelId}")
    suspend fun delete(@Path("channelId") channelId: String)

    @POST("v1/channels/{channelId}/follow")
    suspend fun follow(@Path("channelId") channelId: String): ChannelFollowResponseDto

    @POST("v1/channels/follow/{inviteCode}")
    suspend fun followByInvite(@Path("inviteCode") inviteCode: String): ChannelFollowResponseDto

    @DELETE("v1/channels/{channelId}/follow")
    suspend fun unfollow(@Path("channelId") channelId: String)

    @PUT("v1/channels/{channelId}/mute")
    suspend fun setMuted(@Path("channelId") channelId: String, @Body body: SetMutedBody)

    @GET("v1/channels/{channelId}/admins")
    suspend fun listAdmins(@Path("channelId") channelId: String): ChannelAdminsResponseDto

    @POST("v1/channels/{channelId}/admins/{userId}")
    suspend fun addAdmin(@Path("channelId") channelId: String, @Path("userId") userId: String)

    @DELETE("v1/channels/{channelId}/admins/{userId}")
    suspend fun removeAdmin(@Path("channelId") channelId: String, @Path("userId") userId: String)

    @POST("v1/channels/{channelId}/invite-link")
    suspend fun generateInviteLink(@Path("channelId") channelId: String): ChannelInviteLinkResponseDto

    @DELETE("v1/channels/{channelId}/invite-link")
    suspend fun revokeInviteLink(@Path("channelId") channelId: String)

    @POST("v1/channels/{channelId}/report")
    suspend fun report(@Path("channelId") channelId: String, @Body body: ChannelReportBody)

    @POST("v1/channels/{channelId}/posts")
    suspend fun createPost(@Path("channelId") channelId: String, @Body body: CreatePostBody): ChannelPostDto

    @GET("v1/channels/{channelId}/posts")
    suspend fun listPosts(@Path("channelId") channelId: String, @Query("cursor") cursor: String? = null): ChannelPostsResponseDto

    @PATCH("v1/channels/posts/{postId}")
    suspend fun editPost(@Path("postId") postId: String, @Body body: EditPostBody): ChannelPostDto

    @DELETE("v1/channels/posts/{postId}")
    suspend fun deletePost(@Path("postId") postId: String)

    @POST("v1/channels/posts/{postId}/view")
    suspend fun viewPost(@Path("postId") postId: String)

    @POST("v1/channels/posts/{postId}/reactions")
    suspend fun react(@Path("postId") postId: String, @Body body: StoryReactBody)

    @DELETE("v1/channels/posts/{postId}/reactions")
    suspend fun unreact(@Path("postId") postId: String)

    @POST("v1/channels/posts/{postId}/vote")
    suspend fun vote(@Path("postId") postId: String, @Body body: VoteBody)
}
