package com.chatplatform.feature.chat

import android.content.Context
import android.media.MediaRecorder
import android.os.Build
import java.io.File

data class RecordedVoiceNote(val file: File, val mimeType: String, val durationMs: Long)

/**
 * Records to audio/mp4 (AAC-in-MP4) — matches one of the backend's allowed
 * voice MIME types (see backend/internal/media/service.go allowedMimeTypes).
 * Caller is responsible for requesting RECORD_AUDIO permission before
 * calling start().
 */
class VoiceRecorder(private val context: Context) {
    private var recorder: MediaRecorder? = null
    private var outputFile: File? = null
    private var startedAtMs: Long = 0L

    val isRecording: Boolean get() = recorder != null

    fun start(): Result<Unit> = runCatching {
        check(recorder == null) { "already recording" }

        val file = File(context.cacheDir, "voice_${System.currentTimeMillis()}.m4a")
        val mr = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            MediaRecorder(context)
        } else {
            @Suppress("DEPRECATION")
            MediaRecorder()
        }
        mr.apply {
            setAudioSource(MediaRecorder.AudioSource.MIC)
            setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            setAudioEncodingBitRate(64_000)
            setAudioSamplingRate(44_100)
            setOutputFile(file.absolutePath)
            prepare()
            start()
        }
        recorder = mr
        outputFile = file
        startedAtMs = System.currentTimeMillis()
    }

    /** Returns null if start() was never called or already stopped. */
    fun stop(): RecordedVoiceNote? {
        val mr = recorder ?: return null
        val file = outputFile ?: return null
        val durationMs = System.currentTimeMillis() - startedAtMs

        return try {
            mr.stop()
            RecordedVoiceNote(file = file, mimeType = "audio/mp4", durationMs = durationMs)
        } catch (e: Exception) {
            // stop() throws if start() was called but recording was too
            // short / no data was ever written — treat as "nothing recorded"
            // rather than crashing the send flow.
            file.delete()
            null
        } finally {
            mr.release()
            recorder = null
            outputFile = null
        }
    }

    fun cancel() {
        val mr = recorder ?: return
        try {
            mr.stop()
        } catch (_: Exception) {
            // ignore — we're discarding the recording anyway
        }
        mr.release()
        outputFile?.delete()
        recorder = null
        outputFile = null
    }
}
