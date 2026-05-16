import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import './index.css';
import App from './App.tsx';

// Defaults tuned for this app:
//   - staleTime 30s matches the poll cadence we'll add in Phase 6 (§ 7.2)
//   - refetchOnWindowFocus off — avoids surprise refetches when alt-tabbing
//     in dev; the 30s polling in Phase 6 covers freshness
//   - retry once on transient errors, then surface to the page
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
