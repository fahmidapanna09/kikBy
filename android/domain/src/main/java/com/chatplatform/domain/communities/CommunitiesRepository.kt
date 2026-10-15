package com.chatplatform.domain.communities

enum class CommunityRole { OWNER, ADMIN, MEMBER }

data class CommunityMember(val userId: String, val role: CommunityRole)
data class CommunityGroupRef(val conversationId: String, val name: String, val isAnnouncement: Boolean)

data class Community(
    val id: String,
    val name: String,
    val description: String?,
    val discoverable: Boolean,
    val memberCount: Long,
    val groups: List<CommunityGroupRef>,
    /** Only populated when the caller is an admin/owner — mirrors the
     *  backend's own scoping, so there's no "did I have permission to see
     *  this" check needed client-side. */
    val inviteCode: String? = null,
) {
    val announcementConversationId: String? get() = groups.firstOrNull { it.isAnnouncement }?.conversationId
}

data class JoinedCommunity(val communityId: String, val announcementConversationId: String?)

class CommunityActionException(message: String) : Exception(message)

interface CommunitiesRepository {
    suspend fun create(name: String, description: String?, discoverable: Boolean): Community
    /** Communities the caller is a member of — for the "Communities" tab. */
    suspend fun mine(): List<Community>
    /** Discoverable communities matching a search — for finding new ones. */
    suspend fun search(query: String): List<Community>
    suspend fun get(communityId: String): Community
    suspend fun update(communityId: String, name: String?, description: String?, discoverable: Boolean?): Community

    suspend fun joinDiscoverable(communityId: String): JoinedCommunity
    suspend fun joinByInvite(inviteCode: String): JoinedCommunity
    suspend fun leave(communityId: String)

    suspend fun listMembers(communityId: String): List<CommunityMember>
    suspend fun setMemberRole(communityId: String, userId: String, role: CommunityRole)
    suspend fun removeMember(communityId: String, userId: String)
    suspend fun ban(communityId: String, userId: String)
    suspend fun unban(communityId: String, userId: String)

    /** New group, created inside the community. */
    suspend fun createGroup(communityId: String, name: String, description: String?): CommunityGroupRef
    /** Link a group you already own/admin into the community. */
    suspend fun addExistingGroup(communityId: String, conversationId: String)
    suspend fun removeGroup(communityId: String, conversationId: String)

    suspend fun generateInviteCode(communityId: String): String
    suspend fun revokeInviteCode(communityId: String)

    suspend fun report(communityId: String, reason: String)
}
