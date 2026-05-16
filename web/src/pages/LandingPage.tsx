import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, ApiClientError } from '../api/client';
import { setToken } from '../lib/auth';

// Landing page (spec § 7.1 + the auth change): a dropdown of existing profiles;
// selecting one reveals a password box below it. There's also an inline
// "I'm not on the list" flow that creates a new participant. Both paths set the
// auth token (= participant id) and route to the profile.
export function LandingPage() {
  const navigate = useNavigate();

  const {
    data: participants,
    isPending,
    error,
  } = useQuery({
    queryKey: ['participants'],
    queryFn: api.getParticipants,
  });

  // Selected profile from the dropdown — once non-empty, the password box shows.
  const [selectedId, setSelectedId] = useState('');
  const [password, setPassword] = useState('');

  const loginMutation = useMutation({
    mutationFn: ({ id, pw }: { id: string; pw: string }) => api.login(id, pw),
    onSuccess: (resp) => {
      setToken(resp.token);
      navigate(`/profile/${resp.token}`);
    },
  });

  // "I'm not on the list" create flow.
  const [showAdd, setShowAdd] = useState(false);
  const [name, setName] = useState('');
  const createMutation = useMutation({
    mutationFn: (displayName: string) => api.createParticipant(displayName),
    onSuccess: (participant) => {
      // New accounts have no password yet — the token is just the id, so the
      // creator is logged in for this session. An operator assigns a password
      // afterwards so they can log in again later / on another device.
      setToken(participant.id);
      navigate(`/profile/${participant.id}`);
    },
  });

  const trimmedName = name.trim();
  const canCreate = trimmedName.length >= 2 && !createMutation.isPending;
  const canLogin = selectedId !== '' && password !== '' && !loginMutation.isPending;

  const handleLogin = () => {
    if (canLogin) loginMutation.mutate({ id: selectedId, pw: password });
  };

  return (
    <main className="mx-auto max-w-md px-4 py-10 space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Who are you?</h1>
        <p className="mt-1 text-sm text-zinc-400">
          Pick your profile to start predicting.
        </p>
      </div>

      {isPending && <p className="text-sm text-zinc-500">Loading…</p>}

      {error && (
        <p className="text-sm text-red-400">
          Couldn&rsquo;t load profiles: {error.message}
        </p>
      )}

      {participants && participants.length === 0 && (
        <p className="text-sm text-zinc-500">No profiles yet — add yours below.</p>
      )}

      {participants && participants.length > 0 && (
        <div className="space-y-3">
          <select
            value={selectedId}
            onChange={(e) => {
              setSelectedId(e.target.value);
              setPassword('');
              loginMutation.reset();
            }}
            className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100"
          >
            <option value="" disabled>
              Select your profile…
            </option>
            {participants.map((p) => (
              <option key={p.id} value={p.id}>
                {p.display_name}
              </option>
            ))}
          </select>

          {/* Password box — appears once a profile is chosen. */}
          {selectedId !== '' && (
            <div className="space-y-2">
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleLogin();
                }}
                placeholder="Password"
                autoFocus
                className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100"
              />
              <button
                type="button"
                onClick={handleLogin}
                disabled={!canLogin}
                className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
              >
                {loginMutation.isPending ? 'Logging in…' : 'Log in'}
              </button>
              {loginMutation.isError && (
                <p className="text-sm text-red-400">
                  {loginMutation.error instanceof ApiClientError &&
                  loginMutation.error.code === 'invalid_credentials'
                    ? 'Incorrect password.'
                    : loginMutation.error instanceof Error
                      ? loginMutation.error.message
                      : 'Something went wrong.'}
                </p>
              )}
            </div>
          )}
        </div>
      )}

      {/* Create-new-profile flow. */}
      {!showAdd ? (
        <button
          type="button"
          onClick={() => setShowAdd(true)}
          className="text-sm text-blue-400 hover:text-blue-300"
        >
          I&rsquo;m not on the list
        </button>
      ) : (
        <div className="space-y-2">
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && canCreate) createMutation.mutate(trimmedName);
            }}
            placeholder="Your name"
            maxLength={40}
            autoFocus
            className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100"
          />
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => canCreate && createMutation.mutate(trimmedName)}
              disabled={!canCreate}
              className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
            >
              {createMutation.isPending ? 'Creating…' : 'Submit'}
            </button>
            <button
              type="button"
              onClick={() => {
                setShowAdd(false);
                setName('');
                createMutation.reset();
              }}
              className="text-sm text-zinc-400 hover:text-zinc-200"
            >
              Cancel
            </button>
          </div>
          {createMutation.isError && (
            <p className="text-sm text-red-400">
              {createMutation.error instanceof ApiClientError
                ? createMutation.error.code === 'id_collision'
                  ? 'That name is already taken — try another.'
                  : createMutation.error.message
                : 'Something went wrong.'}
            </p>
          )}
        </div>
      )}
    </main>
  );
}
