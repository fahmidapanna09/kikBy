package com.chatplatform.feature.groups

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.chat.ChatRepository
import com.chatplatform.domain.groups.GroupActionException
import com.chatplatform.domain.groups.GroupsRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CreateGroupState(
    val name: String = "",
    val description: String = "",
    val isCreating: Boolean = false,
    val errorMessage: String? = null,
    /** Set once the group exists — the screen navigates into it. */
    val createdConversationId: String? = null,
)

class CreateGroupViewModel(
    private val groups: GroupsRepository,
    private val chat: ChatRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(CreateGroupState())
    val state: StateFlow<CreateGroupState> = _state.asStateFlow()

    fun onNameChanged(value: String) = _state.update { it.copy(name = value.take(60), errorMessage = null) }
    fun onDescriptionChanged(value: String) = _state.update { it.copy(description = value.take(200)) }

    fun create(memberUserIds: List<String>) {
        val current = _state.value
        if (current.isCreating) return
        if (current.name.isBlank()) {
            _state.update { it.copy(errorMessage = "Give the group a name.") }
            return
        }
        if (memberUserIds.isEmpty()) {
            _state.update { it.copy(errorMessage = "Pick at least one person to add.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isCreating = true, errorMessage = null) }
            try {
                val group = groups.createGroup(current.name.trim(), current.description.trim().ifEmpty { null }, memberUserIds)
                runCatching { chat.refreshConversations() } // so it's in the list before we navigate
                _state.update { it.copy(isCreating = false, createdConversationId = group.conversationId) }
            } catch (e: GroupActionException) {
                _state.update { it.copy(isCreating = false, errorMessage = e.message) }
            }
        }
    }
}
