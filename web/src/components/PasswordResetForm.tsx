import { useId, useState } from 'react';
import { useMutation } from '@tanstack/react-query';

import { api } from '../api/client';

interface PasswordResetFormProps {
  participantId: string;
}

// PasswordResetForm expands account recovery and reports when the new password is saved.
export function PasswordResetForm({ participantId }: PasswordResetFormProps) {
  const formId = useId();
  const passwordId = useId();
  const [expanded, setExpanded] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const resetMutation = useMutation({
    mutationFn: (password: string) => api.resetPassword(participantId, password),
    onSuccess: () => setNewPassword(''),
  });

  const canReset = newPassword.trim() !== '' && !resetMutation.isPending;

  return (
    <div className="pt-1">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={formId}
        disabled={resetMutation.isPending}
        onClick={() => {
          setExpanded(!expanded);
          setNewPassword('');
          resetMutation.reset();
        }}
        className="text-xs text-blue-400 underline underline-offset-2 hover:text-blue-300 disabled:opacity-50"
      >
        Forgot Password?
      </button>

      {expanded && (
        <form
          id={formId}
          className="mt-3 space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            if (canReset) resetMutation.mutate(newPassword);
          }}
        >
          <label htmlFor={passwordId} className="block text-sm text-zinc-400">
            Reset your password to anything
          </label>
          <input
            id={passwordId}
            type="password"
            autoComplete="new-password"
            placeholder="New password"
            maxLength={1024}
            value={newPassword}
            disabled={resetMutation.isPending}
            onChange={(event) => {
              setNewPassword(event.target.value);
              resetMutation.reset();
            }}
            className="w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-sm text-zinc-100"
          />
          <button
            type="submit"
            disabled={!canReset}
            className="rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
          >
            {resetMutation.isPending ? 'Resetting…' : 'Reset Password'}
          </button>
          {resetMutation.isSuccess && (
            <p role="status" className="text-sm text-emerald-400">
              Password reset. Log in above with your new password.
            </p>
          )}
          {resetMutation.isError && (
            <p role="alert" className="text-sm text-red-400">
              {resetMutation.error.message}
            </p>
          )}
        </form>
      )}
    </div>
  );
}
