package com.chatplatform.feature.communities

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.communities.CommunitiesRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CreateCommunityState(
    val name: String = "",
    val description: String = "",
    val discoverable: Boolean = false,
    val isCreating: Boolean = false,
    val errorMessage: String? = null,
    val createdId: String? = null,
)

class CreateCommunityViewModel(private val repository: CommunitiesRepository) : ViewModel() {

    private val _state = MutableStateFlow(CreateCommunityState())
    val state: StateFlow<CreateCommunityState> = _state.asStateFlow()

    fun onNameChanged(v: String) = _state.update { it.copy(name = v.take(80), errorMessage = null) }
    fun onDescriptionChanged(v: String) = _state.update { it.copy(description = v.take(300)) }
    fun onDiscoverableChanged(v: Boolean) = _state.update { it.copy(discoverable = v) }

    fun create() {
        val s = _state.value
        if (s.name.isBlank()) {
            _state.update { it.copy(errorMessage = "Give the community a name.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isCreating = true, errorMessage = null) }
            try {
                val community = repository.create(s.name.trim(), s.description.trim().ifEmpty { null }, s.discoverable)
                _state.update { it.copy(isCreating = false, createdId = community.id) }
            } catch (e: Exception) {
                _state.update { it.copy(isCreating = false, errorMessage = "Couldn't create community: ${e.message}") }
            }
        }
    }
}
