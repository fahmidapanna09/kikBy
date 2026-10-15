package com.chatplatform.core.data.db

import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey

/**
 * clientTmpId is the primary key — not the server id — because it's the one
 * value known at the moment the user hits "send", before any network round
 * trip. This same row is later updated in place with serverId/seq once the
 * server acks it, rather than being replaced, so a message never
 * "disappears and reappears" in the UI during a slow network.
 */
@Entity(
    tableName = "messages",
    indices = [Index(value = ["conversationId", "seq"]), Index(value = ["conversationId", "status"])],
)
data class MessageEntity(
    @PrimaryKey val clientTmpId: String,
    val serverId: String?,       // null until the server acks/echoes this message
    val conversationId: String,
    val senderId: String,
    val type: String,
    val body: String?,
    val replyToId: String?,
    val seq: Long?,              // null until acked; used for ordering once known
    val createdAtEpochMs: Long,  // client-side timestamp, used for ordering PENDING messages before they have a seq
    val status: MessageStatus,
    val failureReason: String? = null,
    /** Comma-joined media_files ids (never contains commas itself — they're
     *  UUIDs — so no escaping needed). Empty string, not null, when there
     *  are no attachments, to keep the Room column NOT NULL-friendly. */
    val attachmentIdsCsv: String = "",
    /** Set when this message is a forward of another message — drives the
     *  "Forwarded" label in the UI. Null for ordinary messages. */
    val forwardedFromMessageId: String? = null,
) {
    val attachmentIds: List<String>
        get() = if (attachmentIdsCsv.isBlank()) emptyList() else attachmentIdsCsv.split(",")
}

fun List<String>.toCsv(): String = joinToString(",")

enum class MessageStatus { PENDING, SENT, DELIVERED, READ, FAILED }
