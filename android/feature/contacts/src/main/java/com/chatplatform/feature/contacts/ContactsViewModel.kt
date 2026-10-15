package com.chatplatform.feature.contacts

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.core.data.PhoneHasher
import com.chatplatform.domain.contacts.Contact
import com.chatplatform.domain.contacts.ContactsRepository
import com.chatplatform.domain.contacts.PublicProfile
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class ContactsUiState(
    val contacts: List<Contact> = emptyList(),
    val isLoading: Boolean = false,
    val isSyncing: Boolean = false,
    val errorMessage: String? = null,
    val searchQuery: String = "",
    val searchResults: List<PublicProfile> = emptyList(),
)

class ContactsViewModel(private val repository: ContactsRepository) : ViewModel() {

    private val _uiState = MutableStateFlow(ContactsUiState())
    val uiState: StateFlow<ContactsUiState> = _uiState.asStateFlow()
    private var searchJob: Job? = null

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _uiState.update { it.copy(isLoading = true, errorMessage = null) }
            try {
                val contacts = repository.listContacts()
                _uiState.update { it.copy(isLoading = false, contacts = contacts) }
            } catch (e: Exception) {
                _uiState.update { it.copy(isLoading = false, errorMessage = "Couldn't load contacts: ${e.message}") }
            }
        }
    }

    fun addContact(phoneNumber: String, nickname: String?) {
        viewModelScope.launch {
            try {
                repository.addContact(phoneNumber, nickname)
                refresh()
            } catch (e: Exception) {
                _uiState.update { it.copy(errorMessage = "Couldn't add contact: ${e.message}") }
            }
        }
    }

    fun deleteContact(contactId: String) {
        viewModelScope.launch {
            try {
                repository.deleteContact(contactId)
                refresh()
            } catch (e: Exception) {
                _uiState.update { it.copy(errorMessage = "Couldn't delete contact: ${e.message}") }
            }
        }
    }

    fun renameContact(contactId: String, nickname: String) {
        viewModelScope.launch {
            try {
                repository.updateNickname(contactId, nickname)
                refresh()
            } catch (e: Exception) {
                _uiState.update { it.copy(errorMessage = "Couldn't rename contact: ${e.message}") }
            }
        }
    }

    fun blockContact(userId: String) {
        viewModelScope.launch {
            try {
                repository.blockUser(userId)
            } catch (e: Exception) {
                _uiState.update { it.copy(errorMessage = "Couldn't block user: ${e.message}") }
            }
        }
    }

    /** Call after the caller has already obtained READ_CONTACTS and read
     *  the device address book (see DeviceContactsReader) — this function
     *  only does the hashing + network round trip. */
    fun syncDeviceContacts(phoneNumbers: List<String>) {
        viewModelScope.launch {
            _uiState.update { it.copy(isSyncing = true, errorMessage = null) }
            try {
                val hashes = phoneNumbers.map { PhoneHasher.sha256Hex(it) }
                val matches = repository.syncByHashes(hashes)
                _uiState.update { it.copy(isSyncing = false, searchResults = matches) }
            } catch (e: Exception) {
                _uiState.update { it.copy(isSyncing = false, errorMessage = "Sync failed: ${e.message}") }
            }
        }
    }

    fun onSearchQueryChanged(value: String) {
        _uiState.update { it.copy(searchQuery = value) }
        searchJob?.cancel()
        if (value.isBlank()) {
            _uiState.update { it.copy(searchResults = emptyList()) }
            return
        }
        // Debounced: username search is rate-limited server-side (30/min per
        // user — see backend/internal/users/service.go), so firing a
        // request on every keystroke would burn through that budget fast.
        searchJob = viewModelScope.launch {
            delay(300)
            try {
                val results = repository.searchByUsername(value)
                _uiState.update { it.copy(searchResults = results) }
            } catch (e: Exception) {
                _uiState.update { it.copy(errorMessage = "Search failed: ${e.message}") }
            }
        }
    }

    fun dismissError() {
        _uiState.update { it.copy(errorMessage = null) }
    }
}
