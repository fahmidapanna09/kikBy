package com.chatplatform.feature.communities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.communities.Community
import com.chatplatform.domain.communities.CommunitiesRepository
import com.chatplatform.domain.communities.CommunityActionException
import com.chatplatform.domain.communities.CommunityMember
import com.chatplatform.domain.communities.CommunityRole
import com.chatplatform.domain.contacts.ContactNameProvider
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** A group of yours that isn't linked into this community yet — offered as
 *  an "add existing group" option. Supply via [CommunityDetailViewModel]'s
 *  myGroupsProvider at the app's DI/nav layer (it needs the chat module's
 *  conversation list, which this feature module doesn't depend on directly
 *  — see the Communities README note). */
data class GroupOption(val conversationId: String, val name: String)

data class CommunityDetailState(
    val community: Community? = null,
    val labels: Map<String, String> = emptyMap(),
    val myRole: CommunityRole = CommunityRole.MEMBER,
    val members: List<CommunityMember> = emptyList(),
    val availableGroupsToAdd: List<GroupOption> = emptyList(),
    val isLoading: Boolean = true,
    val errorMessage: String? = null,
    val hasLeft: Boolean = false,
)

class CommunityDetailViewModel(
    private val repository: CommunitiesRepository,
    private val names: ContactNameProvider,
    private val communityId: String,
    private val currentUserId: String,
    private val myGroupsProvider: suspend () -> List<GroupOption> = { emptyList() },
) : ViewModel() {

    private val _state = MutableStateFlow(CommunityDetailState())
    val state: StateFlow<CommunityDetailState> = _state.asStateFlow()

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            try {
                val community = repository.get(communityId)
                val members = repository.listMembers(communityId)
                val labels = members.associate { it.userId to names.labelFor(it.userId) }
                val mine = members.firstOrNull { it.userId == currentUserId }?.role ?: CommunityRole.MEMBER
                _state.update { it.copy(community = community, members = members, labels = labels, myRole = mine, isLoading = false) }
            } catch (e: CommunityActionException) {
                _state.update { it.copy(isLoading = false, errorMessage = e.message) }
            }
        }
    }

    fun loadAvailableGroups() {
        viewModelScope.launch {
            val linked = _state.value.community?.groups?.map { it.conversationId }?.toSet() ?: emptySet()
            val options = myGroupsProvider().filterNot { it.conversationId in linked }
            _state.update { it.copy(availableGroupsToAdd = options) }
        }
    }

    fun createGroup(name: String, onDone: () -> Unit) = act({
        repository.createGroup(communityId, name, null)
    }, onDone)

    fun addExistingGroup(conversationId: String, onDone: () -> Unit) = act({
        repository.addExistingGroup(communityId, conversationId)
    }, onDone)

    fun removeGroup(conversationId: String) = act({ repository.removeGroup(communityId, conversationId) })

    fun promote(userId: String) = act({ repository.setMemberRole(communityId, userId, CommunityRole.ADMIN) })
    fun demote(userId: String) = act({ repository.setMemberRole(communityId, userId, CommunityRole.MEMBER) })
    fun removeMember(userId: String) = act({ repository.removeMember(communityId, userId) })
    fun banMember(userId: String) = act({ repository.ban(communityId, userId) })
    fun unbanMember(userId: String) = act({ repository.unban(communityId, userId) })

    fun generateInviteCode() {
        viewModelScope.launch {
            try {
                repository.generateInviteCode(communityId)
                refresh() // Get() returns the invite code for admins/owner — refresh picks it up
            } catch (e: CommunityActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun revokeInviteCode() = act({ repository.revokeInviteCode(communityId) })

    fun leave() {
        viewModelScope.launch {
            try {
                repository.leave(communityId)
                _state.update { it.copy(hasLeft = true) }
            } catch (e: CommunityActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }

    fun report(reason: String) = act({ repository.report(communityId, reason) })

    fun dismissError() = _state.update { it.copy(errorMessage = null) }

    private fun act(block: suspend () -> Unit, onDone: () -> Unit = {}) {
        viewModelScope.launch {
            try {
                block()
                refresh()
                onDone()
            } catch (e: CommunityActionException) {
                _state.update { it.copy(errorMessage = e.message) }
            }
        }
    }
}
