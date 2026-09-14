import { createContext, useContext } from 'react';
import type { Tournament } from '../types/api';

export interface EventContextValue {
  event: Tournament;
  events: Tournament[];
  selectEvent: (id: number) => void;
  eventSearch: string;
}

export const EventContext = createContext<EventContextValue | null>(null);

export function useEvent(): EventContextValue {
  const value = useContext(EventContext);
  if (!value) throw new Error('Event context is missing');
  return value;
}
