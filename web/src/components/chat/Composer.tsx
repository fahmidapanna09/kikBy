import { useEffect, useRef, useState } from "react";
import { IconButton } from "@/components/common/Button";
import { toast } from "@/components/common/Toast";
import { useTypingEmitter } from "@/hooks/useTypingEmitter";
import { Icon } from "@/icons/Icon";
import { cn, formatBytes } from "@/lib/utils";
import { services } from "@/services";
import { errorMessage } from "@/services/errors";
import { getDraft, setDraft } from "@/state/drafts";
import { editMessage, sendMessage } from "@/state/messages";
import { useApp } from "@/state/store";
import type { Attachment, Conversation, Message } from "@/types/models";

const EMOJI = ["😀", "😂", "😍", "🥰", "😎", "🤔", "😢", "😡", "👍", "👎", "🙏", "👏", "🔥", "🎉", "❤️", "💯", "🙌", "😅", "🤝", "👀", "✅", "🚀", "😴", "🤗"];
const MAX = 4000;

interface Pending {
  key: string;
  name: string;
  size: number;
  progress: number;
  attachment?: Attachment;
  error?: string;
  ctrl: AbortController;
}

interface Props {
  conv: Conversation;
  replyTo: Message | null;
  replyToName?: string;
  editing: Message | null;
  onCancelContext: () => void;
}

export function Composer({ conv, replyTo, replyToName, editing, onCancelContext }: Props) {
  const enterToSend = useApp((s) => s.settings?.chat.enterToSend ?? true);
  const spellcheck = useApp((s) => s.settings?.chat.spellcheck ?? true);
  const [text, setText] = useState(() => getDraft(conv.id));
  const [pending, setPending] = useState<Pending[]>([]);
  const [emojiOpen, setEmojiOpen] = useState(false);
  const area = useRef<HTMLTextAreaElement | null>(null);
  const file = useRef<HTMLInputElement | null>(null);
  const { typed, stop } = useTypingEmitter(conv.id);
  const prevConv = useRef(conv.id);

  // Swap drafts when switching conversation.
  useEffect(() => {
    if (prevConv.current !== conv.id) {
      setDraft(prevConv.current, text);
      prevConv.current = conv.id;
      setText(getDraft(conv.id));
      setPending([]);
    }
  }, [conv.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Entering edit mode loads the message text; leaving restores the draft.
  const editId = editing?.id;
  useEffect(() => {
    if (editing) {
      setDraft(conv.id, text.trim() ? text : getDraft(conv.id));
      setText(editing.text);
      area.current?.focus();
    }
  }, [editId]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (replyTo) area.current?.focus();
  }, [replyTo?.id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-grow up to ~6 lines.
  useEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 140)}px`;
  }, [text]);

  // Persist the draft as the user types (not while editing).
  useEffect(() => {
    if (!editing) setDraft(conv.id, text);
  }, [text, editing, conv.id]);

  const uploading = pending.some((p) => !p.attachment && !p.error);
  const ready = pending.filter((p) => p.attachment).map((p) => p.attachment as Attachment);
  const canSend = !uploading && (text.trim().length > 0 || ready.length > 0) && text.length <= MAX;

  const submit = async () => {
    if (!canSend) return;
    stop();
    if (editing) {
      const next = text.trim();
      if (!next) return;
      if (next === editing.text) return onCancelContext();
      try {
        await editMessage(editing, next);
        setText(getDraft(conv.id));
        onCancelContext();
      } catch (e) {
        toast.error(errorMessage(e, "Couldn't edit the message."));
      }
      return;
    }
    sendMessage({ conversationId: conv.id, text, replyToId: replyTo?.id, attachments: ready });
    setText("");
    setDraft(conv.id, "");
    setPending([]);
    setEmojiOpen(false);
    onCancelContext();
    area.current?.focus();
  };

  const onKeyDown = (e: any) => {
    if (e.key === "Escape") {
      if (emojiOpen) setEmojiOpen(false);
      else if (editing || replyTo) {
        if (editing) setText(getDraft(conv.id));
        onCancelContext();
      }
      return;
    }
    if (e.key !== "Enter" || e.isComposing) return;
    const modifier = e.ctrlKey || e.metaKey;
    if ((enterToSend && !e.shiftKey) || (!enterToSend && modifier)) {
      e.preventDefault();
      void submit();
    }
  };

  const queueFile = (f: File, opts?: { voice?: boolean }) => {
    const ctrl = new AbortController();
    const key = `${f.name}-${f.size}-${Date.now()}-${Math.random()}`;
    setPending((p) => [...p, { key, name: f.name, size: f.size, progress: 0, ctrl }]);
    services.uploads
      .upload(f, (pct) => setPending((p) => p.map((x) => (x.key === key ? { ...x, progress: pct } : x))), ctrl.signal, opts)
      .then((attachment) => setPending((p) => p.map((x) => (x.key === key ? { ...x, attachment, progress: 100 } : x))))
      .catch((e) => {
        if (ctrl.signal.aborted) return;
        setPending((p) => p.map((x) => (x.key === key ? { ...x, error: errorMessage(e, "Upload failed") } : x)));
      });
  };
  const addFiles = (files: FileList | null) => {
    if (!files) return;
    for (const f of Array.from(files).slice(0, 5)) queueFile(f);
    if (file.current) file.current.value = "";
  };

  /* ───────── voice notes (MediaRecorder) ───────── */
  const [rec, setRec] = useState<{ startedAt: number; secs: number } | null>(null);
  const recorder = useRef<MediaRecorder | null>(null);
  const recChunks = useRef<Blob[]>([]);
  const recCancelled = useRef(false);
  const canRecord = typeof window !== "undefined" && typeof MediaRecorder !== "undefined" && !!navigator.mediaDevices?.getUserMedia;
  useEffect(() => {
    if (!rec) return;
    const t = setInterval(() => setRec((r) => (r ? { ...r, secs: Math.floor((Date.now() - r.startedAt) / 1000) } : r)), 250);
    return () => clearInterval(t);
  }, [!!rec]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => { recCancelled.current = true; if (recorder.current?.state === "recording") recorder.current.stop(); }, []);

  const startRecording = async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const mr = new MediaRecorder(stream);
      recChunks.current = [];
      recCancelled.current = false;
      mr.ondataavailable = (e) => e.data.size && recChunks.current.push(e.data);
      mr.onstop = () => {
        stream.getTracks().forEach((t) => t.stop());
        setRec(null);
        if (recCancelled.current || !recChunks.current.length) return;
        const mime = (mr.mimeType || "audio/webm").split(";")[0];
        const ext = mime.includes("ogg") ? "ogg" : mime.includes("mp4") ? "m4a" : "webm";
        queueFile(new File(recChunks.current, `voice-${new Date().toISOString().slice(11, 19).replace(/:/g, "")}.${ext}`, { type: mime }), { voice: true });
      };
      recorder.current = mr;
      mr.start();
      setRec({ startedAt: Date.now(), secs: 0 });
    } catch {
      toast.error("Couldn't use the microphone. Check the browser's permission for this site.");
    }
  };
  const stopRecording = (cancel: boolean) => {
    recCancelled.current = cancel;
    if (recorder.current?.state === "recording") recorder.current.stop();
    else setRec(null);
  };
  const fmtSecs = (n: number) => `${Math.floor(n / 60)}:${String(n % 60).padStart(2, "0")}`;

  const insertEmoji = (e: string) => {
    const el = area.current;
    if (!el) return setText((t) => t + e);
    const s = el.selectionStart ?? text.length;
    const en = el.selectionEnd ?? text.length;
    setText(text.slice(0, s) + e + text.slice(en));
    requestAnimationFrame(() => {
      el.focus();
      el.setSelectionRange(s + e.length, s + e.length);
    });
  };

  return (
    <div className="composer-wrap">
      {(replyTo || editing) && (
        <div className="reply-preview">
          <Icon name={editing ? "edit" : "reply"} size={16} />
          <div className="rp-text">
            <div className="who">{editing ? "Editing message" : `Replying to ${replyToName ?? "message"}`}</div>
            <div className="snip truncate">{(editing ?? replyTo)?.text || "Attachment"}</div>
          </div>
          <IconButton label="Cancel" onClick={() => { if (editing) setText(getDraft(conv.id)); onCancelContext(); }}><Icon name="close" size={16} /></IconButton>
        </div>
      )}
      {pending.length > 0 && (
        <ul className="pending-list" aria-label="Attachments">
          {pending.map((p) => (
            <li key={p.key} className={cn("pending", p.error && "err")}>
              <Icon name="attach" size={16} />
              <span className="truncate">{p.name}</span>
              <span className="pending-meta">{p.error ?? (p.attachment ? formatBytes(p.size) : `${p.progress}%`)}</span>
              {!p.attachment && !p.error && <span className="spinner" aria-hidden="true" style={{ width: 12, height: 12 }} />}
              <button type="button" className="icon-btn" style={{ width: 24, height: 24 }} aria-label={`Remove ${p.name}`} onClick={() => { p.ctrl.abort(); setPending((l) => l.filter((x) => x.key !== p.key)); }}><Icon name="close" size={14} /></button>
            </li>
          ))}
        </ul>
      )}
      {emojiOpen && (
        <div className="emoji-panel" role="group" aria-label="Emoji">
          {EMOJI.map((e) => <button key={e} type="button" onClick={() => insertEmoji(e)} aria-label={`Insert ${e}`}>{e}</button>)}
        </div>
      )}
      <form className="composer" onSubmit={(e: any) => { e.preventDefault(); void submit(); }}>
        <input ref={file} type="file" multiple hidden onChange={(e: any) => addFiles(e.target.files)} aria-hidden="true" tabIndex={-1} />
        {!editing && !rec && <IconButton label="Attach file" onClick={() => file.current?.click()}><Icon name="attach" /></IconButton>}
        {!editing && !rec && canRecord && text.trim() === "" && <IconButton label="Record voice message" onClick={() => void startRecording()}><Icon name="mic" /></IconButton>}
        {rec && (
          <div className="recording" role="status" aria-label="Recording voice message">
            <span className="rec-dot" aria-hidden="true" />
            <span>Recording {fmtSecs(rec.secs)}</span>
            <IconButton label="Cancel recording" onClick={() => stopRecording(true)}><Icon name="trash" size={18} /></IconButton>
            <IconButton label="Finish recording" onClick={() => stopRecording(false)}><Icon name="check" size={18} /></IconButton>
          </div>
        )}
        <div className="composer-input" style={rec ? { display: "none" } : undefined}>
          <textarea
            ref={area}
            rows={1}
            value={text}
            placeholder={editing ? "Edit message" : "Message"}
            aria-label="Message"
            spellCheck={spellcheck}
            maxLength={MAX + 200}
            onChange={(e: any) => { setText(e.target.value); if (!editing) typed(); }}
            onKeyDown={onKeyDown}
            enterKeyHint={enterToSend ? "send" : "enter"}
          />
          <IconButton label="Emoji" active={emojiOpen} aria-expanded={emojiOpen} onClick={() => setEmojiOpen((o) => !o)}><Icon name="smile" size={20} /></IconButton>
        </div>
        <button type="submit" className="send-btn" disabled={!canSend} aria-label={editing ? "Save edit" : "Send message"}>
          <Icon name={editing ? "check" : "send"} size={18} />
        </button>
      </form>
      {text.length > MAX - 500 && <div className={cn("char-count", text.length > MAX && "over")} aria-live="polite">{text.length}/{MAX}</div>}
    </div>
  );
}
