/** Pure validators. Each returns an error message or "" — so they are trivially unit-testable. */

export const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;
export const USERNAME_RE = /^[a-z0-9_]{3,24}$/;

export const required = (v: string, label: string) => (v.trim() ? "" : `${label} is required`);

export function validateIdentifier(v: string): string {
  const s = v.trim();
  if (!s) return "Enter your email or username";
  if (s.includes("@")) return EMAIL_RE.test(s) ? "" : "Enter a valid email address";
  return USERNAME_RE.test(s.toLowerCase()) ? "" : "Usernames are 3–24 letters, numbers or _";
}

export function validateEmail(v: string): string {
  if (!v.trim()) return "Email is required";
  return EMAIL_RE.test(v.trim()) ? "" : "Enter a valid email address";
}

export function validateUsername(v: string): string {
  if (!v.trim()) return "Username is required";
  return USERNAME_RE.test(v.trim().toLowerCase()) ? "" : "3–24 characters: letters, numbers or _";
}

/** Optional field: empty is valid. Accepts +, digits, spaces, dashes, parentheses; 7–15 digits. */
export function validatePhone(v: string): string {
  const s = v.trim();
  if (!s) return "";
  if (!/^\+?[\d\s\-()]+$/.test(s)) return "Use digits only, with an optional leading +";
  const n = s.replace(/\D/g, "").length;
  return n >= 7 && n <= 15 ? "" : "Enter a valid phone number (7–15 digits)";
}
export const normalizePhone = (v: string) => { const t = v.trim(); return t ? (t.startsWith("+") ? "+" : "") + t.replace(/\D/g, "") : ""; };

export function validateName(v: string): string {
  const s = v.trim();
  if (!s) return "Name is required";
  if (s.length < 2) return "Name is too short";
  if (s.length > 50) return "Name is too long (max 50)";
  return "";
}

export function validatePassword(v: string): string {
  if (!v) return "Password is required";
  if (v.length < 8) return "Use at least 8 characters";
  if (!/[A-Za-z]/.test(v) || !/\d/.test(v)) return "Include at least one letter and one number";
  return "";
}

/** 0 (empty) … 4 (strong) */
export function passwordStrength(v: string): { level: 0 | 1 | 2 | 3 | 4; label: string } {
  if (!v) return { level: 0, label: "" };
  let score = 0;
  if (v.length >= 8) score++;
  if (v.length >= 12) score++;
  if (/[a-z]/.test(v) && /[A-Z]/.test(v)) score++;
  if (/\d/.test(v) && /[^A-Za-z0-9]/.test(v)) score++;
  const level = Math.max(1, Math.min(4, score)) as 1 | 2 | 3 | 4;
  return { level, label: ["", "Weak", "Fair", "Good", "Strong"][level] };
}

export function validateConfirm(pw: string, confirm: string): string {
  if (!confirm) return "Confirm your password";
  return pw === confirm ? "" : "Passwords do not match";
}

export type FieldErrors<K extends string> = Partial<Record<K, string>>;
export const hasErrors = (e: Record<string, string | undefined>) => Object.values(e).some(Boolean);
