package com.chatplatform.core.network

import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.PATCH
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path

data class CreateGroupBody(val name: String, val description: String? = null, val icon_media_id: String? = null, val member_user_ids: List<String>)
data class UpdateGroupBody(val name: String? = null, val description: String? = null, val icon_media_id: String? = null)
data class AddMembersBody(val user_ids: List<String>)
data class UpdateMemberRoleBody(val role: String) // "admin" | "member"
data class MemberDto(val user_id: String, val role: String, val joined_at: String)
data class GroupSummaryDto(
    val conversation_id: String,
    val name: String,
    val description: String?,
    val icon_media_id: String?,
    val who_can_send: String,
    val who_can_add_members: String,
    val who_can_edit_info: String,
    val members: List<MemberDto>,
    val invite_code: String?,
)
data class InviteLinkResponseDto(val invite_code: String)
data class JoinByInviteResponseDto(val conversation_id: String)

interface GroupApi {
    @POST("v1/groups")
    suspend fun createGroup(@Body body: CreateGroupBody): GroupSummaryDto

    @GET("v1/groups/{conversationId}")
    suspend fun getGroup(@Path("conversationId") conversationId: String): GroupSummaryDto

    @PATCH("v1/groups/{conversationId}")
    suspend fun updateGroup(@Path("conversationId") conversationId: String, @Body body: UpdateGroupBody): GroupSummaryDto

    @POST("v1/groups/{conversationId}/members")
    suspend fun addMembers(@Path("conversationId") conversationId: String, @Body body: AddMembersBody)

    @DELETE("v1/groups/{conversationId}/members/{userId}")
    suspend fun removeMember(@Path("conversationId") conversationId: String, @Path("userId") userId: String)

    @PUT("v1/groups/{conversationId}/members/{userId}/role")
    suspend fun updateMemberRole(@Path("conversationId") conversationId: String, @Path("userId") userId: String, @Body body: UpdateMemberRoleBody)

    @POST("v1/groups/{conversationId}/invite-link")
    suspend fun generateInviteLink(@Path("conversationId") conversationId: String): InviteLinkResponseDto

    @DELETE("v1/groups/{conversationId}/invite-link")
    suspend fun revokeInviteLink(@Path("conversationId") conversationId: String)

    @POST("v1/groups/join/{inviteCode}")
    suspend fun joinViaInvite(@Path("inviteCode") inviteCode: String): JoinByInviteResponseDto
}
