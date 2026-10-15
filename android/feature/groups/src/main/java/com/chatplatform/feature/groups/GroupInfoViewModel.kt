package com.chatplatform.feature.groups

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.chat.ChatRepository
import com.chatplatform.domain.contacts.ContactNameProvider
import com.chatplatform.domain.groups.GroupActionException
import com.chatplatform.domain.groups.GroupInfo
import com.chatplatform.domain.groups.GroupsRepository
import com.chatplatform.domain.groups.MemberRole
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class GroupInfoState(
    val group: GroupInfo? = null,
    /** userId -> what to call them (contact name, display name, @username). */
    val labels: Map<String, String> = emptyMap(),
    val myRole: MemberRole = MemberRole.MEMBER,
    /** Only known right after generating one — the server never returns an
     *  existing code when you merely load the group (see the backend's
     *  groupSummary), so a code generated earlier isn't shown again here. */
    val inviteCode: String? = null,
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
    val hasLeft: Boolean = false,
)

class GroupInfoViewModel(
    private val groups: GroupsRepository,
    private val names: ContactNameProvider,
    private val chat: ChatRepository,
    private val conversationId: String,
    private val currentUserId: String,
) : ViewModel() {

    private val _state = MutableStateFlow(GroupInfoState())
    val state: StateFlow<GroupInfoState> = _state.asStateFlow()

    /** For building the add-members picker's exclusion list. */
    val currentMemberIds: Set<String> get() = _state.value.group?.members?.map { it.userId }?.toSet() ?: emptySet()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            try {
                val group = groups.getGroup(conversationId)
                val labels = group.members.associate { it.userId to names.labelFor(it.userId) }
                val mine = group.members.firstOrNull { it.userId == currentUserId }?.role ?: MemberRole.MEMBER
                _state.update { it.copy(group = group, labels = labels, myRole = mine, isLoading = false) }
            } catch (e: GroupActionException) {
                _state.update { it.copy(isLoading = false, errorMessage = e.message) }
            }
        }
    }

    fun makeAdmin(userId: String) = act { groups.setMemberRole(conversationId, userId, MemberRole.ADMIN) }
    fun removeAdmin(userId: String) = act { groups.setMemberRole(conversationId, userId, MemberRole.MEMBER) }
    fun removeMember(userId: String) = act { groups.removeMember(conversationId, userId) }

    fun addMembers(userIds: List<String>, onSuccess: () -> Unit) {
        if (userIds.isEmpty()) return
        viewModelScope.launch {
            try {
                groups.addMembers(conversationId, userIds)
                names.refresh()
                refresh()
                onSuccess()
            } catch (e: GroupActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun generateInviteCode() {
        viewModelScope.launch {
            try {
                val code = groups.generateInviteCode(conversationId)
                _state.update { it.copy(inviteCode = code) }
            } catch (e: GroupActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun revokeInviteCode() {
        viewModelScope.launch {
            try {
                groups.revokeInviteCode(conversationId)
                _state.update { it.copy(inviteCode = null) }
            } catch (e: GroupActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun leave() {
        viewModelScope.launch {
            try {
                groups.removeMember(conversationId, currentUserId)
                runCatching { chat.refreshConversations() } // drops it from the local list
                _state.update { it.copy(hasLeft = true) }
            } catch (e: GroupActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }

    private fun act(block: suspend () -> Unit) {
        viewModelScope.launch {
            try {
                block()
                refresh()
            } catch (e: GroupActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }
}
