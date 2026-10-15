package com.chatplatform.data.groups

import com.chatplatform.core.network.AddMembersBody
import com.chatplatform.core.network.CreateGroupBody
import com.chatplatform.core.network.GroupApi
import com.chatplatform.core.network.GroupSummaryDto
import com.chatplatform.core.network.UpdateMemberRoleBody
import com.chatplatform.domain.groups.GroupActionException
import com.chatplatform.domain.groups.GroupInfo
import com.chatplatform.domain.groups.GroupMember
import com.chatplatform.domain.groups.GroupsRepository
import com.chatplatform.domain.groups.MemberRole
import retrofit2.HttpException
import java.io.IOException

class GroupsRepositoryImpl(private val api: GroupApi) : GroupsRepository {

    override suspend fun createGroup(name: String, description: String?, memberUserIds: List<String>): GroupInfo = call {
        api.createGroup(CreateGroupBody(name = name, description = description, member_user_ids = memberUserIds)).toDomain()
    }

    override suspend fun getGroup(conversationId: String): GroupInfo = call { api.getGroup(conversationId).toDomain() }

    override suspend fun addMembers(conversationId: String, userIds: List<String>) = call {
        api.addMembers(conversationId, AddMembersBody(userIds))
    }

    override suspend fun removeMember(conversationId: String, userId: String) = call {
        api.removeMember(conversationId, userId)
    }

    override suspend fun setMemberRole(conversationId: String, userId: String, role: MemberRole) = call {
        api.updateMemberRole(conversationId, userId, UpdateMemberRoleBody(role.name.lowercase()))
    }

    override suspend fun generateInviteCode(conversationId: String): String = call {
        api.generateInviteLink(conversationId).invite_code
    }

    override suspend fun revokeInviteCode(conversationId: String) = call { api.revokeInviteLink(conversationId) }

    override suspend fun joinViaInviteCode(code: String): String = call { api.joinViaInvite(code).conversation_id }

    /** Turns transport/HTTP failures into a sentence the UI can show as-is. */
    private suspend fun <T> call(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw GroupActionException(
            when (e.code()) {
                403 -> "You don't have permission to do that."
                404 -> "That group or invite link wasn't found."
                else -> "Something went wrong (${e.code()})."
            }
        )
    } catch (e: IOException) {
        throw GroupActionException("Couldn't reach the server. Check your connection.")
    }
}

private fun GroupSummaryDto.toDomain() = GroupInfo(
    conversationId = conversation_id,
    name = name,
    description = description,
    members = members.map { m ->
        GroupMember(
            userId = m.user_id,
            role = when (m.role) {
                "owner" -> MemberRole.OWNER
                "admin" -> MemberRole.ADMIN
                else -> MemberRole.MEMBER
            },
        )
    },
)
