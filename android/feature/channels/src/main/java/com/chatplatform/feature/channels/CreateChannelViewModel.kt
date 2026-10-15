package com.chatplatform.feature.channels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.chatplatform.domain.channels.ChannelsRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class CreateChannelState(
    val name: String = "",
    val description: String = "",
    val category: String = "",
    val isPublic: Boolean = true,
    val isCreating: Boolean = false,
    val errorMessage: String? = null,
    val createdId: String? = null,
)

class CreateChannelViewModel(private val repository: ChannelsRepository) : ViewModel() {

    private val _state = MutableStateFlow(CreateChannelState())
    val state: StateFlow<CreateChannelState> = _state.asStateFlow()

    fun onNameChanged(v: String) = _state.update { it.copy(name = v.take(80), errorMessage = null) }
    fun onDescriptionChanged(v: String) = _state.update { it.copy(description = v.take(300)) }
    fun onCategoryChanged(v: String) = _state.update { it.copy(category = v.take(40)) }
    fun onIsPublicChanged(v: Boolean) = _state.update { it.copy(isPublic = v) }

    fun create() {
        val s = _state.value
        if (s.name.isBlank()) {
            _state.update { it.copy(errorMessage = "Give the channel a name.") }
            return
        }
        viewModelScope.launch {
            _state.update { it.copy(isCreating = true, errorMessage = null) }
            try {
                val channel = repository.create(s.name.trim(), s.description.trim().ifEmpty { null }, s.category.trim().ifEmpty { null }, s.isPublic)
                _state.update { it.copy(isCreating = false, createdId = channel.id) }
            } catch (e: Exception) {
                _state.update { it.copy(isCreating = false, errorMessage = "Couldn't create channel: ${e.message}") }
            }
        }
    }
}
