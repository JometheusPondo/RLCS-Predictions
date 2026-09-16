import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useMutation } from '@tanstack/react-query';
import { api } from '../api/client';
import { OWNER_ID, setToken } from '../lib/auth';
import { useEvent } from '../lib/events';

export function AdminLoginPage() {
  const [username, setUsername] = useState(OWNER_ID);
  const [password, setPassword] = useState('');
  const { eventSearch } = useEvent();
  const navigate = useNavigate();
  const login = useMutation({
    mutationFn: () => {
      if (username.trim() !== OWNER_ID) throw new Error('Incorrect username or password.');
      return api.login(username.trim(), password);
    },
    onSuccess: (response) => {
      setToken(response.token);
      navigate(`/leaderboard${eventSearch}`);
    },
  });

  return (
    <main className="mx-auto max-w-md space-y-6 px-4 py-10">
      <h1 className="text-2xl font-semibold tracking-tight">Admin Login</h1>
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!login.isPending) login.mutate(); }}>
        <label className="block space-y-2 text-sm">
          <span>Username</span>
          <input required autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2" />
        </label>
        <label className="block space-y-2 text-sm">
          <span>Password</span>
          <input required type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2" />
        </label>
        <button disabled={login.isPending} className="rounded-md bg-blue-600 px-4 py-2 text-sm hover:bg-blue-500 disabled:opacity-50">{login.isPending ? 'Logging in…' : 'Log in'}</button>
        {login.isError && <p role="alert" className="text-sm text-red-400">Couldn’t log in. Check your username and password and try again.</p>}
      </form>
      <Link to={`/${eventSearch}`} className="text-sm text-zinc-400 underline">Back to profiles</Link>
    </main>
  );
}
