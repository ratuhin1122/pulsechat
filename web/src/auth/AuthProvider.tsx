import { createContext, useContext, useEffect, useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';
import type { ApiClient } from '../api/client';
import type { Session } from './session';

export interface Services { session: Session; api: ApiClient }
const AuthContext = createContext<Services | null>(null);

export function AuthProvider({ services, children }: { services: Services; children: ReactNode }) {
  useEffect(() => {
    const restore = () => { void services.session.restore(); };
    restore();
    window.addEventListener('focus', restore);
    window.addEventListener('online', restore);
    return () => {
      window.removeEventListener('focus', restore);
      window.removeEventListener('online', restore);
    };
  }, [services]);
  return <AuthContext.Provider value={services}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const services = useContext(AuthContext);
  if (!services) throw new Error('AuthProvider is required.');
  const snapshot = useSyncExternalStore(services.session.subscribe, services.session.getSnapshot);
  return { ...services, ...snapshot };
}
