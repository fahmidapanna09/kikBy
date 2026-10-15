import { ServiceError } from "@/services/errors";
import { handleUnauthorized, session } from "@/services/session";
import { sleep } from "@/lib/utils";
import { db, seedFor } from "./db";
import type { MockState } from "./db";

/** Simulated latency + optional failure injection: localStorage.setItem("kikbite.mock.fail","1") makes reads fail (to QA error states). */
export async function delay(min = 120, max = 380): Promise<void> {
  await sleep(min + Math.random() * (max - min));
}

export function shouldFail(): boolean {
  try {
    return localStorage.getItem("kikbite.mock.fail") === "1";
  } catch {
    return false;
  }
}

export async function readDelay() {
  await delay();
  if (shouldFail()) throw new ServiceError("server_error", "The server had a problem (simulated). Please retry.", { status: 500 });
}

/** Mimics server auth: no valid token → 401 → global handler clears session. */
export function requireState(): MockState {
  const s = session.get();
  if (!s || !session.isValid()) {
    handleUnauthorized();
    throw new ServiceError("unauthorized", "Your session has expired. Please sign in again.", { status: 401 });
  }
  seedFor(s.user);
  return db();
}
