import { useEffect, useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { ApiError, errorMessage } from '../api/http';
import { useAuth } from '../auth/AuthProvider';
import { Brand } from '../components/Brand';
import { Icon } from '../components/Icon';

export function AuthPage({ mode }: { mode: 'login' | 'register' }) {
  const { session } = useAuth();
  const register = mode === 'register';
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => { document.title = `${register ? 'Create an account' : 'Sign in'} · Pulsechat`; }, [register]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setError('');
    const bytes = (value: string) => new TextEncoder().encode(value).length;
    if (register && !username.trim()) {
      setError('Choose a username to get started.'); return;
    }
    if (register && bytes(username.trim()) > 64) {
      setError('That username is too long. Try a shorter one.'); return;
    }
    if (bytes(password) > 1024) {
      setError('That password is too long. Try a shorter one.'); return;
    }
    if (register && password.length < 8) {
      setError('Use at least 8 characters for your password.'); return;
    }
    setBusy(true);
    try {
      const input = { email: email.trim(), password };
      if (register) await session.register({ ...input, username: username.trim() });
      else await session.login(input);
      setPassword('');
    } catch (error) {
      setError(error instanceof ApiError && error.status === 401
        ? 'Email or password is incorrect.' : errorMessage(error));
    } finally { setBusy(false); }
  }

  return <div className="auth-layout">
    <section className="auth-panel" aria-labelledby="auth-title">
      <Brand />
      <div className="auth-content">
        <span className="eyebrow">YOUR PEOPLE. YOUR PLACE.</span>
        <h1 id="auth-title">{register ? 'Make yourself at home.' : 'Welcome back.'}</h1>
        <p className="muted">{register ? 'Create an account and find your conversations.' : 'A good conversation is just a sign-in away.'}</p>
        <form className="auth-form" onSubmit={(event) => { void submit(event); }} aria-busy={busy}>
          {register && <label>Username
            <input name="username" autoComplete="username" required maxLength={64} value={username}
              onChange={(event) => setUsername(event.target.value)} disabled={busy} />
          </label>}
          <label>Email address
            <input name="email" type="email" autoComplete="email" inputMode="email" required maxLength={254}
              placeholder="you@example.com" value={email} onChange={(event) => setEmail(event.target.value)} disabled={busy} />
          </label>
          <label htmlFor="password">Password</label>
          <div className="password-field">
            <input id="password" name="password" type={visible ? 'text' : 'password'} required
              minLength={register ? 8 : undefined} maxLength={1024} autoComplete={register ? 'new-password' : 'current-password'}
              value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy}
              aria-describedby={register ? 'password-hint' : undefined} />
            <button type="button" className="password-toggle" aria-pressed={visible}
              aria-label={visible ? 'Hide password' : 'Show password'} onClick={() => setVisible(!visible)} disabled={busy}>
              {visible ? 'Hide' : 'Show'}
            </button>
          </div>
          {register && <small className="muted" id="password-hint">At least 8 characters. Make it yours.</small>}
          {error && <p className="error-message" role="alert">{error}</p>}
          <button className="primary-button" type="submit" disabled={busy}>
            {busy ? (register ? 'Creating account…' : 'Signing in…') : (register ? 'Create account' : 'Sign in')}
            {!busy && <Icon name="arrow" />}
          </button>
        </form>
        <p className="auth-switch">{register ? 'Already have an account? ' : 'New around here? '}
          <Link to={register ? '/login' : '/register'}>{register ? 'Sign in' : 'Create an account'}</Link>
        </p>
      </div>
      <p className="auth-footnote">A little closer, wherever you are.</p>
    </section>
    <aside className="auth-story" aria-labelledby="story-title">
      <div className="story-label"><span /> A SPACE TO CONNECT</div>
      <div className="conversation-art" aria-hidden="true">
        <div className="art-orbit orbit-one" /><div className="art-orbit orbit-two" />
        <div className="art-bubble bubble-front"><span /><span /><span /></div>
        <div className="art-bubble bubble-back"><span /><span /><span /></div>
        <div className="art-spark spark-one" /><div className="art-spark spark-two" />
      </div>
      <div className="story-copy"><h2 id="story-title">Good conversations<br />start here.</h2>
        <p>One place for your people, your rooms,<br className="desktop-break" /> and the moments in between.</p>
      </div>
      <p className="story-footer">LESS DISTANCE. MORE CONNECTION.</p>
    </aside>
  </div>;
}
