package com.chatplatform.core.data.db

import androidx.room.Database
import androidx.room.RoomDatabase
import androidx.room.TypeConverter
import androidx.room.TypeConverters

class Converters {
    @TypeConverter
    fun statusToString(status: MessageStatus): String = status.name

    @TypeConverter
    fun stringToStatus(value: String): MessageStatus = MessageStatus.valueOf(value)
}

@Database(
    entities = [MessageEntity::class, ConversationEntity::class],
    version = 1,
    exportSchema = true, // keep schema history under app/schemas/ for migration testing later
)
@TypeConverters(Converters::class)
abstract class AppDatabase : RoomDatabase() {
    abstract fun messageDao(): MessageDao
    abstract fun conversationDao(): ConversationDao
}
