import { Link, useNavigate } from 'react-router-dom';

import { clearToken, useAuth } from '../lib/auth';
import { useEvent } from '../lib/events';

// Header renders on every route. When logged in it shows a direct link to your
// own profile plus a Log out action; when anonymous it shows "Switch Profile"
// which just routes to the landing page's profile picker.
export function Header() {
  const { event, events, selectEvent, eventSearch } = useEvent();
  const token = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    clearToken();
    navigate(`/${eventSearch}`);
  };

  return (
    <header className="border-b border-zinc-800 bg-zinc-950">
      <div className="mx-auto max-w-5xl px-4 py-3 flex flex-wrap gap-4 items-center justify-between">
        <Link
          to={`/${eventSearch}`}
          className="flex items-center gap-2 text-lg font-semibold tracking-tight text-zinc-100 hover:text-white"
        >
          <img src="/logos/rlcs-logo.png" alt="" className="h-8 w-8 shrink-0 object-contain" />
          RLCS Predictions
        </Link>
        <label className="flex items-center gap-2 text-sm text-zinc-400">
          Event
          <select aria-label="Event" value={event.id} onChange={(e) => selectEvent(Number(e.target.value))} className="max-w-full rounded-md border border-zinc-700 bg-zinc-900 px-3 py-2 text-zinc-100">
            {events.map((item) => <option key={item.id} value={item.id}>{item.name}{item.is_active ? '' : ' · Archive'}</option>)}
          </select>
        </label>
        <nav className="flex items-center gap-4 text-sm text-zinc-300">
          <Link to={`/leaderboard${eventSearch}`} className="hover:text-white">
            Leaderboard
          </Link>
          {token ? (
            <>
              <Link to={`/profile/${token}${eventSearch}`} className="hover:text-white">
                My Picks
              </Link>
              <button type="button" onClick={handleLogout} className="hover:text-white">
                Log out
              </button>
            </>
          ) : (
            <Link to={`/${eventSearch}`} className="hover:text-white">
              Switch Profile
            </Link>
          )}
        </nav>
      </div>
    </header>
  );
}
