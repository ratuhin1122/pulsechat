type IconName = 'trash' | 'rooms' | 'search' | 'arrow' | 'logout' | 'menu' | 'close' | 'refresh' | 'message' | 'hash';

export function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, string> = {
    trash: 'M3 6h18 M9 6V3h6v3 M5 6l1 15h12l1-15 M10 10v7 M14 10v7',
    rooms: 'M4 5h16v12H9l-5 4V5Z M8 9h8 M8 13h5',
    search: 'M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0',
    arrow: 'M5 12h14 M13 6l6 6-6 6',
    logout: 'M9 4H4v16h5 M10 12h11 M16 7l5 5-5 5',
    menu: 'M4 6h16 M4 12h16 M4 18h16',
    close: 'M6 6l12 12 M6 18 18 6',
    refresh: 'M20 10a8 8 0 1 0 0 5 M20 4v6h-6',
    message: 'M5 4h14a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2h-8l-6 3v-3H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z M8 9h8 M8 13h5',
    hash: 'M10 3 8 21 M17 3l-2 18 M4 9h17 M3 15h17',
  };
  return <svg className="icon" viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false">
    <path d={paths[name]} stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" />
  </svg>;
}
