import { Navigate, Route, Routes } from 'react-router-dom';
import { useAuth } from './auth/AuthProvider';
import { Brand } from './components/Brand';
import { AuthPage } from './pages/AuthPage';
import { Workspace } from './pages/Workspace';

export function App() {
  const { status, error, session } = useAuth();
  if (status === 'checking' || status === 'unavailable') {
    return <main className="session-screen"><Brand /><div className="session-card" role="status">
      {status === 'checking' ? <><span className="spinner" aria-hidden="true" /><h1>Getting things ready…</h1><p>Finding your space.</p></>
        : <><h1>Let’s try that again.</h1><p>{error}</p><button className="primary-button" onClick={() => { void session.restore(); }}>Retry connection</button></>}
    </div></main>;
  }
  const authenticated = status === 'authenticated';
  return <Routes>
    <Route path="/login" element={authenticated ? <Navigate to="/" replace /> : <AuthPage key="login" mode="login" />} />
    <Route path="/register" element={authenticated ? <Navigate to="/" replace /> : <AuthPage key="register" mode="register" />} />
    <Route path="/" element={authenticated ? <Workspace /> : <Navigate to="/login" replace />} />
    <Route path="*" element={<Navigate to="/" replace />} />
  </Routes>;
}
