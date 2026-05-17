import { useEffect, useState } from 'react';

// Auth state for the honor-system tool. The "token" is literally the
// participant id — there's no session table on the server (see the
// conversation spec). localStorage persists it across refreshes; a custom
// event keeps every mounted component's useAuth() in sync after login/logout
// within the tab, and the native 'storage' event syncs across tabs.

const TOKEN_KEY = 'rlcs_token';
const AUTH_EVENT = 'rlcs-auth-change';

// ADMIN_ID is the backstage reference account. It logs in through the normal
// landing-page dropdown (so it must appear there), but is filtered out of the
// leaderboard. Must stay in sync with models.AdminID on the Go side.
export const ADMIN_ID = 'blast_admin';

// LOCK_EXEMPT_IDS are the special, non-standard leaderboard accounts — "The
// Coin" (a coin-flip benchmark) and "Chat" (a chat-vote benchmark) — whose
// picks the operator may set or change at any time, including after a match
// has locked for regular participants. They are scored and ranked like every
// other participant; the prediction lock is the only rule they skip.
//
// Must stay in sync with lockExemptParticipants in internal/db/queries.go.
export const LOCK_EXEMPT_IDS: readonly string[] = ['the-coin', 'chat'];

// isLockExempt reports whether a participant id bypasses prediction locking.
export function isLockExempt(id: string | null): boolean {
  return id !== null && LOCK_EXEMPT_IDS.includes(id);
}

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
  window.dispatchEvent(new Event(AUTH_EVENT));
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
  window.dispatchEvent(new Event(AUTH_EVENT));
}

// useAuth returns the current participant id (the token), or null if not
// logged in. Re-renders the calling component whenever auth changes — in this
// tab (AUTH_EVENT) or another (storage event).
export function useAuth(): string | null {
  const [token, setTokenState] = useState<string | null>(getToken);

  useEffect(() => {
    const sync = () => setTokenState(getToken());
    window.addEventListener(AUTH_EVENT, sync);
    window.addEventListener('storage', sync);
    return () => {
      window.removeEventListener(AUTH_EVENT, sync);
      window.removeEventListener('storage', sync);
    };
  }, []);

  return token;
}
