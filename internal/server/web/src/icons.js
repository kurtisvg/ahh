// Small inline SVGs keep navigation icons consistent without an icon font.
export function icon(name) {
  const paths = {
    conversations: '<path d="M21 14a3 3 0 0 1-3 3H8l-5 4V6a3 3 0 0 1 3-3h12a3 3 0 0 1 3 3Z"/><path d="M7 8h10M7 12h6"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    folder: '<path d="M3 7V5a2 2 0 0 1 2-2h5l3 4h6a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>',
  };
  const teeth = Array.from({ length: 8 }, (_, tooth) => [-18, -12, 12, 18].map((offset, corner) => {
    const angle = (tooth * 45 + offset) * Math.PI / 180;
    const radius = corner === 0 || corner === 3 ? 7.8 : 10;
    return `${12 + radius * Math.cos(angle)},${12 + radius * Math.sin(angle)}`;
  })).flat();
  paths.settings = `<path d="M${teeth.join('L')}Z"/><circle cx="12" cy="12" r="3"/>`;
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  svg.innerHTML = paths[name];
  return svg;
}
