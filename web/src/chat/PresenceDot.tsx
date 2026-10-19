export function PresenceDot({ online }: { online: boolean | undefined }) {
  const label = online === undefined ? 'Presence unknown' : online ? 'Online' : 'Offline';
  return <span role="img" aria-label={label} title={label}
    className={`presence-dot ${online === undefined ? 'is-unknown' : online ? 'is-online' : 'is-offline'}`} />;
}
