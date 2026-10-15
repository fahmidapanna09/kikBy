package com.chatplatform.data.communities

import com.chatplatform.core.network.CommunitiesApi
import com.chatplatform.core.network.CommunityGroupRefDto
import com.chatplatform.core.network.CommunityJoinResponseDto
import com.chatplatform.core.network.CommunityMemberDto
import com.chatplatform.core.network.CommunityReportBody
import com.chatplatform.core.network.CommunitySummaryDto
import com.chatplatform.core.network.CommunityUpdateRoleBody
import com.chatplatform.core.network.CreateCommunityBody
import com.chatplatform.core.network.CreateGroupInCommunityBody
import com.chatplatform.core.network.UpdateCommunityBody
import com.chatplatform.domain.communities.Community
import com.chatplatform.domain.communities.CommunityActionException
import com.chatplatform.domain.communities.CommunitiesRepository
import com.chatplatform.domain.communities.CommunityGroupRef
import com.chatplatform.domain.communities.CommunityMember
import com.chatplatform.domain.communities.CommunityRole
import com.chatplatform.domain.communities.JoinedCommunity
import retrofit2.HttpException
import java.io.IOException

class CommunitiesRepositoryImpl(private val api: CommunitiesApi) : CommunitiesRepository {

    override suspend fun create(name: String, description: String?, discoverable: Boolean): Community =
        call { api.create(CreateCommunityBody(name, description, discoverable = discoverable)).toDomain() }

    override suspend fun mine(): List<Community> =
        call { api.mine().communities?.map { it.toDomain() } ?: emptyList() }

    override suspend fun search(query: String): List<Community> =
        call { api.search(query).communities?.map { it.toDomain() } ?: emptyList() }

    override suspend fun get(communityId: String): Community = call { api.get(communityId).toDomain() }

    override suspend fun update(communityId: String, name: String?, description: String?, discoverable: Boolean?): Community =
        call { api.update(communityId, UpdateCommunityBody(name, description, discoverable = discoverable)).toDomain() }

    override suspend fun joinDiscoverable(communityId: String): JoinedCommunity = call { api.joinDiscoverable(communityId).toDomain() }
    override suspend fun joinByInvite(inviteCode: String): JoinedCommunity = call { api.joinByInvite(inviteCode).toDomain() }
    override suspend fun leave(communityId: String) = call { api.leave(communityId) }

    override suspend fun listMembers(communityId: String): List<CommunityMember> =
        call { api.listMembers(communityId).members?.map { it.toDomain() } ?: emptyList() }

    override suspend fun setMemberRole(communityId: String, userId: String, role: CommunityRole) =
        call { api.updateMemberRole(communityId, userId, CommunityUpdateRoleBody(role.name.lowercase())) }

    override suspend fun removeMember(communityId: String, userId: String) = call { api.removeMember(communityId, userId) }
    override suspend fun ban(communityId: String, userId: String) = call { api.ban(communityId, userId) }
    override suspend fun unban(communityId: String, userId: String) = call { api.unban(communityId, userId) }

    override suspend fun createGroup(communityId: String, name: String, description: String?): CommunityGroupRef = call {
        val dto = api.createGroup(communityId, CreateGroupInCommunityBody(name, description))
        CommunityGroupRef(dto.conversation_id, dto.name, isAnnouncement = false)
    }

    override suspend fun addExistingGroup(communityId: String, conversationId: String) = call { api.addExistingGroup(communityId, conversationId) }
    override suspend fun removeGroup(communityId: String, conversationId: String) = call { api.removeGroup(communityId, conversationId) }

    override suspend fun generateInviteCode(communityId: String): String = call { api.generateInviteLink(communityId).invite_code }
    override suspend fun revokeInviteCode(communityId: String) = call { api.revokeInviteLink(communityId) }

    override suspend fun report(communityId: String, reason: String) = call { api.report(communityId, CommunityReportBody(reason)) }

    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw CommunityActionException(
            when (e.code()) {
                403 -> "You don't have permission to do that."
                404 -> "That community, group, or invite link wasn't found."
                else -> "Something went wrong (${e.code()})."
            }
        )
    } catch (e: IOException) {
        throw CommunityActionException("Couldn't reach the server. Check your connection.")
    }
}

private fun roleOf(s: String) = when (s) {
    "owner" -> CommunityRole.OWNER
    "admin" -> CommunityRole.ADMIN
    else -> CommunityRole.MEMBER
}

private fun CommunitySummaryDto.toDomain() = Community(
    id = id, name = name, description = description, discoverable = discoverable, memberCount = member_count,
    groups = (groups ?: emptyList()).map { it.toDomain() }, inviteCode = invite_code,
)

private fun CommunityGroupRefDto.toDomain() = CommunityGroupRef(conversation_id, name, is_announcement)
private fun CommunityMemberDto.toDomain() = CommunityMember(user_id, roleOf(role))
private fun CommunityJoinResponseDto.toDomain() = JoinedCommunity(community_id, announcement_conversation_id)
