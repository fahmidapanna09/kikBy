package com.chatplatform.feature.groups

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.contacts.ContactsRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class PickableMember(val userId: String, val label: String, val subtitle: String? = null)

data class MemberPickerState(
    val query: String = "",
    /** The viewer's saved contacts who are registered users (only those can
     *  be added to a group — an unregistered phone number has no account). */
    val contacts: List<PickableMember> = emptyList(),
    /** Username-search hits; shown instead of `contacts` while `query` is non-blank. */
    val searchResults: List<PickableMember> = emptyList(),
    /** userId -> label for everyone currently ticked, kept separately so a
     *  selection made from a search result survives clearing the query. */
    val selected: Map<String, String> = emptyMap(),
    val isLoading: Boolean = false,
    val errorMessage: String? = null,
)

/**
 * Shared by "create group" and "add members". [excludedUserIds] are people
 * who must not appear at all — for create that's just yourself, for
 * add-members it's everyone already in the group.
 */
class MemberPickerViewModel(
    private val contactsRepository: ContactsRepository,
    private val excludedUserIds: Set<String>,
) : ViewModel() {

    private val _state = MutableStateFlow(MemberPickerState(isLoading = true))
    val state: StateFlow<MemberPickerState> = _state.asStateFlow()
    private var searchJob: Job? = null

    val selectedUserIds: List<String> get() = _state.value.selected.keys.toList()

    init {
        loadContacts()
    }

    private fun loadContacts() {
        viewModelScope.launch {
            try {
                val members = contactsRepository.listContacts()
                    .filter { it.isRegistered && it.contactUserId != null && it.contactUserId !in excludedUserIds }
                    .map { PickableMember(it.contactUserId!!, it.displayLabel, it.username?.let { u -> "@$u" }) }
                _state.update { it.copy(isLoading = false, contacts = members) }
            } catch (e: Exception) {
                _state.update { it.copy(isLoading = false, errorMessage = "Couldn't load your contacts.") }
            }
        }
    }

    fun onQueryChanged(value: String) {
        _state.update { it.copy(query = value) }
        searchJob?.cancel()
        if (value.isBlank()) {
            _state.update { it.copy(searchResults = emptyList()) }
            return
        }
        // Debounced — username search is rate-limited server-side.
        searchJob = viewModelScope.launch {
            delay(300)
            try {
                val results = contactsRepository.searchByUsername(value)
                    .filter { it.userId !in excludedUserIds }
                    .map { PickableMember(it.userId, it.displayName ?: "@${it.username}", "@${it.username}") }
                _state.update { it.copy(searchResults = results) }
            } catch (e: Exception) {
                _state.update { it.copy(errorMessage = "Search failed. Try again.") }
            }
        }
    }

    fun toggle(member: PickableMember) {
        _state.update {
            val next = it.selected.toMutableMap()
            if (next.remove(member.userId) == null) next[member.userId] = member.label
            it.copy(selected = next)
        }
    }

    fun dismissError() = _state.update { it.copy(errorMessage = null) }
}
