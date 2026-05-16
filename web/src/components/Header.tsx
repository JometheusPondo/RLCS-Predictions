import { Link, useNavigate } from 'react-router-dom';

import { clearToken, useAuth } from '../lib/auth';

// Header renders on every route. When logged in it shows a direct link to your
// own profile plus a Log out action; when anonymous it shows "Switch Profile"
// which just routes to the landing page's profile picker.
export function Header() {
  const token = useAuth();
  const navigate = useNavigate();

  const handleLogout = () => {
    clearToken();
    navigate('/');
  };

  return (
    <header className="border-b border-zinc-800 bg-zinc-950">
      <div className="mx-auto max-w-5xl px-4 py-3 flex items-center justify-between">
        <Link
          to="/"
          className="text-lg font-semibold tracking-tight text-zinc-100 hover:text-white"
        >
          RLCS Paris Major Predictions
        </Link>
        <nav className="flex items-center gap-4 text-sm text-zinc-300">
          <Link to="/leaderboard" className="hover:text-white">
            Leaderboard
          </Link>
          {token ? (
            <>
              <Link to={`/profile/${token}`} className="hover:text-white">
                My Picks
              </Link>
              <button type="button" onClick={handleLogout} className="hover:text-white">
                Log out
              </button>
            </>
          ) : (
            <Link to="/" className="hover:text-white">
              Switch Profile
            </Link>
          )}
        </nav>
      </div>
    </header>
  );
}
