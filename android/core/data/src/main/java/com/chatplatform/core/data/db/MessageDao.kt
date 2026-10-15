package com.chatplatform.core.data.db

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import androidx.room.Update
import kotlinx.coroutines.flow.Flow

@Dao
interface MessageDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(message: MessageEntity)

    @Update
    suspend fun update(message: MessageEntity)

    @Query("""
        SELECT * FROM messages
        WHERE conversationId = :conversationId
        ORDER BY COALESCE(seq, 9223372036854775807) ASC, createdAtEpochMs ASC
    """)
    fun observeMessages(conversationId: String): Flow<List<MessageEntity>>

    @Query("SELECT * FROM messages WHERE clientTmpId = :clientTmpId LIMIT 1")
    suspend fun getByClientTmpId(clientTmpId: String): MessageEntity?

    @Query("SELECT * FROM messages WHERE serverId = :serverId LIMIT 1")
    suspend fun getByServerId(serverId: String): MessageEntity?

    /** The offline queue: anything still PENDING or FAILED gets retried on
     *  reconnect / app foreground, oldest first so ordering is preserved. */
    @Query("SELECT * FROM messages WHERE status = 'PENDING' OR status = 'FAILED' ORDER BY createdAtEpochMs ASC")
    suspend fun getOutbox(): List<MessageEntity>

    @Query("UPDATE messages SET status = :status, failureReason = :reason WHERE clientTmpId = :clientTmpId")
    suspend fun updateStatus(clientTmpId: String, status: MessageStatus, reason: String? = null)

    @Query("UPDATE messages SET serverId = :serverId, seq = :seq, status = 'SENT' WHERE clientTmpId = :clientTmpId")
    suspend fun applyAck(clientTmpId: String, serverId: String, seq: Long)
}
