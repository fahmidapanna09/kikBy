package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query

data class CreateStoryBody(
    val type: String,
    val text_content: String? = null,
    val background_color: String? = null,
    val caption: String? = null,
    val media_id: String? = null,
    val privacy: String? = null,
    val audience_user_ids: List<String>? = null,
    val mention_user_ids: List<String>? = null,
)

data class StoryDto(
    val id: String,
    val author_id: String,
    val type: String,
    val text_content: String?,
    val background_color: String?,
    val caption: String?,
    val media_id: String?,
    val created_at: String,
    val expires_at: String,
    val seen: Boolean,
    val privacy: String? = null,
    val view_count: Long? = null,
    val reaction_count: Long? = null,
    val reply_count: Long? = null,
)

data class StoryFeedEntryDto(
    val author_id: String,
    val username: String,
    val display_name: String?,
    val muted: Boolean,
    val has_unseen: Boolean,
    val stories: List<StoryDto>,
)
data class StoryFeedResponseDto(val entries: List<StoryFeedEntryDto>?)
data class MyStoriesResponseDto(val stories: List<StoryDto>?)

data class StoryViewerDto(val user_id: String, val username: String, val display_name: String?, val viewed_at: String, val reaction: String?)
data class StoryViewersResponseDto(val viewers: List<StoryViewerDto>?)

data class StoryReactBody(val emoji: String)
data class StoryReplyBody(val body: String)
data class StoryReplyDto(val id: String, val story_id: String, val replier_id: String, val body: String, val created_at: String)
data class StoryRepliesResponseDto(val replies: List<StoryReplyDto>?)

interface StoriesApi {
    @POST("v1/stories")
    suspend fun create(@Body body: CreateStoryBody): StoryDto

    @GET("v1/stories/feed")
    suspend fun feed(): StoryFeedResponseDto

    @GET("v1/stories/mine")
    suspend fun mine(): MyStoriesResponseDto

    @GET("v1/stories/archive")
    suspend fun archive(@Query("before") beforeEpochMs: Long? = null): MyStoriesResponseDto

    @DELETE("v1/stories/{storyId}")
    suspend fun delete(@Path("storyId") storyId: String)

    @POST("v1/stories/{storyId}/view")
    suspend fun view(@Path("storyId") storyId: String)

    @GET("v1/stories/{storyId}/viewers")
    suspend fun viewers(@Path("storyId") storyId: String): StoryViewersResponseDto

    @POST("v1/stories/{storyId}/reactions")
    suspend fun react(@Path("storyId") storyId: String, @Body body: StoryReactBody)

    @DELETE("v1/stories/{storyId}/reactions")
    suspend fun unreact(@Path("storyId") storyId: String)

    @POST("v1/stories/{storyId}/replies")
    suspend fun reply(@Path("storyId") storyId: String, @Body body: StoryReplyBody): StoryReplyDto

    @GET("v1/stories/{storyId}/replies")
    suspend fun replies(@Path("storyId") storyId: String): StoryRepliesResponseDto

    @POST("v1/stories/mute/{userId}")
    suspend fun mute(@Path("userId") userId: String)

    @DELETE("v1/stories/mute/{userId}")
    suspend fun unmute(@Path("userId") userId: String)
}
