package com.chatplatform.core.data.db

import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "conversations")
data class ConversationEntity(
    @PrimaryKey val id: String,
    val type: String,
    val peerUserId: String?,
    val name: String? = null, // group name; null for direct conversations
    val lastMessageClientTmpId: String?,
    val lastMessageAtEpochMs: Long?,
    val unreadCount: Long,
)
