package com.chatplatform.domain.groups

enum class MemberRole { OWNER, ADMIN, MEMBER }

data class GroupMember(val userId: String, val role: MemberRole)

data class GroupInfo(
    val conversationId: String,
    val name: String,
    val description: String?,
    val members: List<GroupMember>,
)

/** Thrown for failures the UI can show as-is (already a friendly sentence). */
class GroupActionException(message: String) : Exception(message)

interface GroupsRepository {
    suspend fun createGroup(name: String, description: String?, memberUserIds: List<String>): GroupInfo
    suspend fun getGroup(conversationId: String): GroupInfo
    suspend fun addMembers(conversationId: String, userIds: List<String>)

    /** Also how a member leaves: pass your own user id. */
    suspend fun removeMember(conversationId: String, userId: String)

    /** role must be ADMIN (promote) or MEMBER (demote); the server enforces
     *  who is allowed to do which. */
    suspend fun setMemberRole(conversationId: String, userId: String, role: MemberRole)

    suspend fun generateInviteCode(conversationId: String): String
    suspend fun revokeInviteCode(conversationId: String)
    suspend fun joinViaInviteCode(code: String): String
}
