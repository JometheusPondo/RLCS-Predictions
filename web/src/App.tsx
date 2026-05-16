import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { Header } from './components/Header';
import { LandingPage } from './pages/LandingPage';
import { LeaderboardPage } from './pages/LeaderboardPage';
import { ProfilePage } from './pages/ProfilePage';

// Three routes per spec § 7. Header renders on every page; routes change below.
// The * catch-all keeps unknown URLs from showing a blank page.
function App() {
  return (
    <BrowserRouter>
      <Header />
      <Routes>
        <Route path="/" element={<LandingPage />} />
        <Route path="/profile/:id" element={<ProfilePage />} />
        <Route path="/leaderboard" element={<LeaderboardPage />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </BrowserRouter>
  );
}

function NotFound() {
  return (
    <main className="mx-auto max-w-5xl px-4 py-12">
      <h1 className="text-2xl font-semibold tracking-tight">404</h1>
      <p className="mt-1 text-sm text-zinc-400">page not found</p>
    </main>
  );
}

export default App;
