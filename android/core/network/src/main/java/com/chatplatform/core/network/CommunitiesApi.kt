package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path
import retrofit2.http.Query

data class CreateCommunityBody(val name: String, val description: String? = null, val icon_media_id: String? = null, val discoverable: Boolean = false)
data class UpdateCommunityBody(val name: String? = null, val description: String? = null, val icon_media_id: String? = null, val discoverable: Boolean? = null)
data class CreateGroupInCommunityBody(val name: String, val description: String? = null)
data class CommunityMemberDto(val user_id: String, val role: String, val joined_at: String)
data class CommunityGroupRefDto(val conversation_id: String, val name: String, val is_announcement: Boolean)
data class CommunitySummaryDto(
    val id: String,
    val name: String,
    val description: String?,
    val icon_media_id: String?,
    val discoverable: Boolean,
    val member_count: Long,
    val groups: List<CommunityGroupRefDto>?,
    val invite_code: String?,
)
data class CommunityInviteLinkResponseDto(val invite_code: String)
data class CommunityJoinResponseDto(val community_id: String, val announcement_conversation_id: String?)
data class CommunitySearchResponseDto(val communities: List<CommunitySummaryDto>?)
data class CommunityMembersResponseDto(val members: List<CommunityMemberDto>?)
data class CommunityUpdateRoleBody(val role: String) // "admin" | "member"
data class CommunityReportBody(val reason: String)
data class GroupRefDto(val conversation_id: String, val name: String)

interface CommunitiesApi {
    @POST("v1/communities")
    suspend fun create(@Body body: CreateCommunityBody): CommunitySummaryDto

    @GET("v1/communities/search")
    suspend fun search(@Query("q") query: String): CommunitySearchResponseDto

    @GET("v1/communities/mine")
    suspend fun mine(): CommunitySearchResponseDto

    @GET("v1/communities/{communityId}")
    suspend fun get(@Path("communityId") communityId: String): CommunitySummaryDto

    @PATCH("v1/communities/{communityId}")
    suspend fun update(@Path("communityId") communityId: String, @Body body: UpdateCommunityBody): CommunitySummaryDto

    @POST("v1/communities/{communityId}/join")
    suspend fun joinDiscoverable(@Path("communityId") communityId: String): CommunityJoinResponseDto

    @POST("v1/communities/join/{inviteCode}")
    suspend fun joinByInvite(@Path("inviteCode") inviteCode: String): CommunityJoinResponseDto

    @POST("v1/communities/{communityId}/leave")
    suspend fun leave(@Path("communityId") communityId: String)

    @GET("v1/communities/{communityId}/members")
    suspend fun listMembers(@Path("communityId") communityId: String): CommunityMembersResponseDto

    @PUT("v1/communities/{communityId}/members/{userId}/role")
    suspend fun updateMemberRole(@Path("communityId") communityId: String, @Path("userId") userId: String, @Body body: CommunityUpdateRoleBody)

    @DELETE("v1/communities/{communityId}/members/{userId}")
    suspend fun removeMember(@Path("communityId") communityId: String, @Path("userId") userId: String)

    @POST("v1/communities/{communityId}/ban/{userId}")
    suspend fun ban(@Path("communityId") communityId: String, @Path("userId") userId: String)

    @DELETE("v1/communities/{communityId}/ban/{userId}")
    suspend fun unban(@Path("communityId") communityId: String, @Path("userId") userId: String)

    @POST("v1/communities/{communityId}/groups")
    suspend fun createGroup(@Path("communityId") communityId: String, @Body body: CreateGroupInCommunityBody): GroupRefDto

    @POST("v1/communities/{communityId}/groups/{conversationId}")
    suspend fun addExistingGroup(@Path("communityId") communityId: String, @Path("conversationId") conversationId: String)

    @DELETE("v1/communities/{communityId}/groups/{conversationId}")
    suspend fun removeGroup(@Path("communityId") communityId: String, @Path("conversationId") conversationId: String)

    @POST("v1/communities/{communityId}/invite-link")
    suspend fun generateInviteLink(@Path("communityId") communityId: String): CommunityInviteLinkResponseDto

    @DELETE("v1/communities/{communityId}/invite-link")
    suspend fun revokeInviteLink(@Path("communityId") communityId: String)

    @POST("v1/communities/{communityId}/report")
    suspend fun report(@Path("communityId") communityId: String, @Body body: CommunityReportBody)
}
