import type { ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';

import { api } from '../api/client';
import { EventContext } from '../lib/events';

export function EventProvider({ children }: { children: ReactNode }) {
  const [params, setParams] = useSearchParams();
  const query = useQuery({ queryKey: ['events'], queryFn: api.getEvents });

  if (query.isPending) return <p className="p-8 text-zinc-400">Loading events…</p>;
  if (query.error) return <p className="p-8 text-red-400">Couldn’t load events: {query.error.message}</p>;

  const events = query.data ?? [];
  const requested = params.get('event');
  const event = requested === null
    ? events.find((item) => item.is_active)
    : events.find((item) => String(item.id) === requested);

  if (!event) {
    return <div className="p-8 space-y-3"><p>Event not found.</p><button className="text-blue-400" onClick={() => setParams({})}>Go to current event</button></div>;
  }

  const selectEvent = (id: number) => {
    const next = new URLSearchParams(params);
    next.set('event', String(id));
    setParams(next);
  };

  return (
    <EventContext.Provider value={{ event, events, selectEvent, eventSearch: `?event=${event.id}` }}>
      {children}
    </EventContext.Provider>
  );
}
