package com.chatplatform.core.data.db

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import kotlinx.coroutines.flow.Flow

@Dao
interface ConversationDao {
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsert(conversation: ConversationEntity)

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAll(conversations: List<ConversationEntity>)

    @Query("SELECT * FROM conversations ORDER BY lastMessageAtEpochMs DESC")
    fun observeAll(): Flow<List<ConversationEntity>>

    /** Called after a full refresh so conversations the user has left (or
     *  that were deleted) disappear locally instead of lingering forever. */
    @Query("DELETE FROM conversations WHERE id NOT IN (:keepIds)")
    suspend fun deleteNotIn(keepIds: List<String>)

    @Query("DELETE FROM conversations")
    suspend fun deleteAll()

    @Query("SELECT * FROM conversations WHERE id = :id LIMIT 1")
    suspend fun getById(id: String): ConversationEntity?
}
