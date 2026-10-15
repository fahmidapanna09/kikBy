import type { ID } from "@/types/models";

/** Unsent composer text per conversation (memory only — survives switching chats, not reloads). */
const drafts = new Map<ID, string>();
export const getDraft = (id: ID) => drafts.get(id) ?? "";
export const setDraft = (id: ID, text: string) => (text ? drafts.set(id, text) : drafts.delete(id));
