import { Link } from 'react-router-dom';

export function Brand() {
  return <Link className="brand" to="/" aria-label="Pulsechat home">
    <span className="brand-mark" aria-hidden="true">
      <svg viewBox="0 0 24 24" fill="none"><path d="M4 12h4l2-5 4 10 2-5h4" /></svg>
    </span>
    pulsechat<span className="brand-period">.</span>
  </Link>;
}
