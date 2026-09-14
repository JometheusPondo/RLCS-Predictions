import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { Header } from './components/Header';
import { LandingPage } from './pages/LandingPage';
import { LeaderboardPage } from './pages/LeaderboardPage';
import { ProfilePage } from './pages/ProfilePage';
import { EventProvider } from './components/EventProvider';
import { useEvent } from './lib/events';

// Three routes per spec § 7. Header renders on every page; routes change below.
// The * catch-all keeps unknown URLs from showing a blank page.
function App() {
  return (
    <BrowserRouter>
      <EventProvider><EventRoutes /></EventProvider>
    </BrowserRouter>
  );
}

function EventRoutes() {
  const { event } = useEvent();
  return (
    <>
      <Header />
      {!event.is_active && (
        <div className="border-b border-amber-900/60 bg-amber-950/30 px-4 py-3 text-center text-sm text-amber-200">
          {event.name} · Archive — predictions are read-only.
        </div>
      )}
      <Routes key={event.id}>
        <Route path="/" element={<LandingPage />} />
        <Route path="/profile/:id" element={<ProfilePage />} />
        <Route path="/leaderboard" element={<LeaderboardPage />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </>
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
