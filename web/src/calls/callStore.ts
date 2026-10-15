import { createStore, useStore } from "@/state/createStore";
import type { ID } from "@/types/models";
import type { ActiveCall, CallKind, IncomingCall } from "./types";

export interface CallState {
  active: ActiveCall | null;
  incoming: IncomingCall | null;
  /** Live (ringing/active) call per conversation, so a chat can show "Join". */
  live: Record<ID, { callId: ID; kind: CallKind }>;
}

export const callStore = createStore<CallState>(() => ({ active: null, incoming: null, live: {} }));
export const useCalls = <S,>(selector: (s: CallState) => S): S => useStore(callStore, selector);
