// Typed API client. One function per endpoint. All requests go through a
// single `request()` helper that handles error envelopes uniformly: any 4xx/5xx
// throws an ApiClientError carrying the status, server-provided code, and
// message — callers (and TanStack Query's error path) can switch on `code` for
// UX decisions (e.g. show "match locked" vs "not found").

import type {
  Match,
  Participant,
  ParticipantWithPredictions,
  Pick,
  Prediction,
  SyncStatus,
  ApiErrorBody,
  LoginResponse,
} from '../types/api';
import { getToken } from '../lib/auth';

const BASE = '/api';

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiClientError';
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // Content-Type is only required on requests with a body. Setting it on GETs
  // is harmless but the Go backend's middleware skips bodyless requests anyway.
  const headers: Record<string, string> = { ...(init?.headers as Record<string, string> | undefined) };
  if (init?.body && !('Content-Type' in headers)) {
    headers['Content-Type'] = 'application/json';
  }

  // Bearer token = the participant id (honor-system auth). Sent on every
  // request when present; the server's auth middleware is permissive, so
  // anonymous requests still work for public endpoints.
  const token = getToken();
  if (token && !('Authorization' in headers)) {
    headers['Authorization'] = `Bearer ${token}`;
  }

  const res = await fetch(`${BASE}${path}`, { ...init, headers });

  if (!res.ok) {
    let body: ApiErrorBody | null = null;
    try {
      body = (await res.json()) as ApiErrorBody;
    } catch {
      // Non-JSON error body (e.g. proxy or network layer); fall through.
    }
    throw new ApiClientError(
      res.status,
      body?.code ?? 'unknown',
      body?.error ?? res.statusText,
    );
  }

  // 204 No Content (currently only DELETE returns this).
  if (res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

export const api = {
  getMatches: (): Promise<Match[]> => request<Match[]>('/matches'),

  getParticipants: (): Promise<Participant[]> => request<Participant[]>('/participants'),

  getParticipant: (id: string): Promise<ParticipantWithPredictions> =>
    request<ParticipantWithPredictions>(`/participants/${encodeURIComponent(id)}`),

  // NOTE: self-registration is currently disabled — POST /api/participants is
  // not registered on the backend, so calling this will 404. Kept for when an
  // approval flow is built; see the router and LandingPage for the other half.
  createParticipant: (display_name: string): Promise<Participant> =>
    request<Participant>('/participants', {
      method: 'POST',
      body: JSON.stringify({ display_name }),
    }),

  setPrediction: (participantId: string, matchId: string, pick: Pick): Promise<Prediction> =>
    request<Prediction>(
      `/participants/${encodeURIComponent(participantId)}/predictions/${encodeURIComponent(matchId)}`,
      { method: 'PUT', body: JSON.stringify({ pick }) },
    ),

  deletePrediction: (participantId: string, matchId: string): Promise<void> =>
    request<void>(
      `/participants/${encodeURIComponent(participantId)}/predictions/${encodeURIComponent(matchId)}`,
      { method: 'DELETE' },
    ),

  getSyncStatus: (): Promise<SyncStatus> => request<SyncStatus>('/sync/status'),

  // login validates participant_id + password. On success the returned token
  // (which is the participant id) should be persisted via lib/auth setToken.
  login: (participantId: string, password: string): Promise<LoginResponse> =>
    request<LoginResponse>('/login', {
      method: 'POST',
      body: JSON.stringify({ participant_id: participantId, password }),
    }),

  // setWinnerPick appends a tournament-winner pick. Auth-gated server-side to
  // the participant themselves or blast_admin. Returns the updated participant.
  setWinnerPick: (participantId: string, teamName: string): Promise<ParticipantWithPredictions> =>
    request<ParticipantWithPredictions>(
      `/participants/${encodeURIComponent(participantId)}/winner`,
      { method: 'PUT', body: JSON.stringify({ team_name: teamName }) },
    ),
};
