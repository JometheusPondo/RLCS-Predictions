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
  SimulationResponse,
  SyncStatus,
  ApiErrorBody,
  LoginResponse,
  Tournament,
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

async function request<T>(path: string, init?: RequestInit, eventId?: number): Promise<T> {
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

  const eventQuery = eventId === undefined ? '' : `?event=${eventId}`;
  const res = await fetch(`${BASE}${path}${eventQuery}`, { ...init, headers });

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

  // Endpoints without a response body return 204 No Content.
  if (res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

export const api = {
  getEvents: (): Promise<Tournament[]> => request<Tournament[]>('/events'),

  getTeams: (eventId: number): Promise<string[]> => request<string[]>('/teams', undefined, eventId),

  getMatches: (eventId: number): Promise<Match[]> => request<Match[]>('/matches', undefined, eventId),

  getParticipants: (eventId: number): Promise<Participant[]> => request<Participant[]>('/participants', undefined, eventId),

  getParticipant: (id: string, eventId: number): Promise<ParticipantWithPredictions> =>
    request<ParticipantWithPredictions>(`/participants/${encodeURIComponent(id)}`, undefined, eventId),

  // getSimulation returns the best-case / worst-case standings projection for
  // the current day. It never alters the leaderboard — the caller overlays the
  // per-participant deltas onto the real, points-sorted board.
  getSimulation: (eventId: number): Promise<SimulationResponse> => request<SimulationResponse>('/simulation', undefined, eventId),

  // NOTE: self-registration is currently disabled — POST /api/participants is
  // not registered on the backend, so calling this will 404. Kept for when an
  // approval flow is built; see the router and LandingPage for the other half.
  createParticipant: (display_name: string): Promise<Participant> =>
    request<Participant>('/participants', {
      method: 'POST',
      body: JSON.stringify({ display_name }),
    }),

  setPrediction: (participantId: string, matchId: string, pick: Pick, eventId: number): Promise<Prediction> =>
    request<Prediction>(
      `/participants/${encodeURIComponent(participantId)}/predictions/${encodeURIComponent(matchId)}`,
      { method: 'PUT', body: JSON.stringify({ pick }) },
      eventId,
    ),

  deletePrediction: (participantId: string, matchId: string, eventId: number): Promise<void> =>
    request<void>(
      `/participants/${encodeURIComponent(participantId)}/predictions/${encodeURIComponent(matchId)}`,
      { method: 'DELETE' },
      eventId,
    ),

  getSyncStatus: (): Promise<SyncStatus> => request<SyncStatus>('/sync/status'),

  // login validates participant_id + password. On success the returned token
  // (which is the participant id) should be persisted via lib/auth setToken.
  login: (participantId: string, password: string): Promise<LoginResponse> =>
    request<LoginResponse>('/login', {
      method: 'POST',
      body: JSON.stringify({ participant_id: participantId, password }),
    }),

  resetPassword: (participantId: string, newPassword: string): Promise<void> =>
    request<void>('/reset-password', {
      method: 'POST',
      body: JSON.stringify({ participant_id: participantId, new_password: newPassword }),
    }),

  // setWinnerPick appends a tournament-winner pick. Auth-gated server-side to
  // the participant themselves or blast_admin. Returns the updated participant.
  setWinnerPick: (participantId: string, teamName: string, eventId: number): Promise<ParticipantWithPredictions> =>
    request<ParticipantWithPredictions>(
      `/participants/${encodeURIComponent(participantId)}/winner`,
      { method: 'PUT', body: JSON.stringify({ team_name: teamName }) },
      eventId,
    ),
};
