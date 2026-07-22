export interface IconProps {
  color?: string;
  size?: number | string;
  strokeWidth?: number;
}

const defaultProps: Required<IconProps> = {
  color: 'currentColor',
  size: 24,
  strokeWidth: 2,
};

function createSvg(paths: string[], props: IconProps = {}): string {
  const { color, size, strokeWidth } = { ...defaultProps, ...props };
  const dimension = typeof size === 'number' ? `${size}` : size;

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${dimension}" height="${dimension}" viewBox="0 0 24 24" fill="none" stroke="${color}" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round">${paths.join('')}</svg>`;
}

export const Globe = (props?: IconProps): string =>
  createSvg(
    [
      '<circle cx="12" cy="12" r="10"/>',
      '<path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/>',
      '<path d="M2 12h20"/>',
    ],
    props
  );

export const Gamepad2 = (props?: IconProps): string =>
  createSvg(
    [
      '<line x1="6" x2="10" y1="11" y2="11"/>',
      '<line x1="8" x2="8" y1="9" y2="13"/>',
      '<line x1="15" x2="15.01" y1="12" y2="12"/>',
      '<line x1="18" x2="18.01" y1="10" y2="10"/>',
      '<path d="M17.32 5H6.68a4 4 0 0 0-3.978 3.59c-.006.052-.01.101-.017.152C2.604 9.416 2 14.456 2 16a3 3 0 0 0 3 3c1 0 1.5-.5 2-1l1.414-1.414A2 2 0 0 1 9.828 16h4.344a2 2 0 0 1 1.414.586L17 18c.5.5 1 1 2 1a3 3 0 0 0 3-3c0-1.545-.604-6.584-.685-7.258-.007-.05-.011-.1-.017-.151A4 4 0 0 0 17.32 5z"/>',
    ],
    props
  );

export const Zap = (props?: IconProps): string =>
  createSvg(
    ['<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>'],
    props
  );

export const UserPlus = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/>',
      '<circle cx="9" cy="7" r="4"/>',
      '<line x1="19" x2="19" y1="8" y2="14"/>',
      '<line x1="22" x2="16" y1="11" y2="11"/>',
    ],
    props
  );

export const ArrowRight = (props?: IconProps): string =>
  createSvg(
    ['<path d="M5 12h14"/>', '<path d="m12 5 7 7-7 7"/>'],
    props
  );

export const Trophy = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6"/>',
      '<path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18"/>',
      '<path d="M4 22h16"/>',
      '<path d="M10 14.66V17c0 .55-.47.98-.97 1.21C7.85 18.75 7 20.24 7 22"/>',
      '<path d="M14 14.66V17c0 .55.47.98.97 1.21C16.15 18.75 17 20.24 17 22"/>',
      '<path d="M18 2H6v7a6 6 0 0 0 12 0V2Z"/>',
    ],
    props
  );

export const MousePointer = (props?: IconProps): string =>
  createSvg(
    ['<path d="M3.347 2.678a1 1 0 0 1 1.33-1.33l17 6a1 1 0 0 1-.025 1.844l-6.39 2.322-2.322 6.39a1 1 0 0 1-1.844.025z"/>'],
    props
  );

export const Timer = (props?: IconProps): string =>
  createSvg(
    [
      '<line x1="10" x2="14" y1="2" y2="2"/>',
      '<line x1="12" x2="12" y1="14" y2="10"/>',
      '<circle cx="12" cy="14" r="8"/>',
    ],
    props
  );

export const Users = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/>',
      '<circle cx="9" cy="7" r="4"/>',
      '<path d="M22 21v-2a4 4 0 0 0-3-3.87"/>',
      '<path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
    ],
    props
  );

export const Award = (props?: IconProps): string =>
  createSvg(
    [
      '<circle cx="12" cy="8" r="6"/>',
      '<path d="M15.477 12.89 17 22l-5-3-5 3 1.523-9.11"/>',
    ],
    props
  );

export const Menu = (props?: IconProps): string =>
  createSvg(
    ['<line x1="4" x2="20" y1="12" y2="12"/>', '<line x1="4" x2="20" y1="6" y2="6"/>', '<line x1="4" x2="20" y1="18" y2="18"/>'],
    props
  );

export const Volume2 = (props?: IconProps): string =>
  createSvg(
    [
      '<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>',
      '<path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>',
      '<path d="M19.07 4.93a10 10 0 0 1 0 14.14"/>',
    ],
    props
  );

export const VolumeX = (props?: IconProps): string =>
  createSvg(
    [
      '<polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>',
      '<line x1="22" x2="16" y1="9" y2="15"/>',
      '<line x1="16" x2="22" y1="9" y2="15"/>',
    ],
    props
  );

export const Wind = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M17.7 7.7a2.5 2.5 0 1 1 1.8 4.3H2"/>',
      '<path d="M9.6 4.6A2 2 0 1 1 11 8H2"/>',
      '<path d="M12.6 19.4A2 2 0 1 0 14 16H2"/>',
    ],
    props
  );

export const ChevronLeft = (props?: IconProps): string =>
  createSvg(['<path d="m15 18-6-6 6-6"/>'], props);

export const ChevronRight = (props?: IconProps): string =>
  createSvg(['<path d="m9 18 6-6-6-6"/>'], props);

export const Mail = (props?: IconProps): string =>
  createSvg(
    [
      '<rect width="20" height="16" x="2" y="4" rx="2"/>',
      '<path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7"/>',
    ],
    props
  );

export const Send = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="m22 2-7 20-4-9-9-4Z"/>',
      '<path d="M22 2 11 13"/>',
    ],
    props
  );

export const LogIn = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"/>',
      '<polyline points="10 17 15 12 10 7"/>',
      '<line x1="15" x2="3" y1="12" y2="12"/>',
    ],
    props
  );

export const LogOut = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>',
      '<polyline points="16 17 21 12 16 7"/>',
      '<line x1="21" x2="9" y1="12" y2="12"/>',
    ],
    props
  );

export const Home = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
      '<polyline points="9 22 9 12 15 12 15 22"/>',
    ],
    props
  );

export const RotateCcw = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/>',
      '<path d="M3 3v5h5"/>',
    ],
    props
  );

export const Play = (props?: IconProps): string =>
  createSvg(
    ['<polygon points="6 3 20 12 6 21 6 3"/>'],
    props
  );

export const Copy = (props?: IconProps): string =>
  createSvg(
    [
      '<rect width="14" height="14" x="8" y="8" rx="2" ry="2"/>',
      '<path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>',
    ],
    props
  );

export const Github = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4"/>',
      '<path d="M9 18c-4.51 2-5-2-7-2"/>',
    ],
    props
  );

export const Twitter = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M22 4s-.7 2.1-2 3.4c1.6 10-9.4 17.3-18 11.6 2.2.1 4.4-.6 6-2C3 15.5.5 9.6 3 5c2.2 2.6 5.6 4.1 9 4-.9-4.2 4-6.6 7-3.8 1.1 0 3-1.2 3-1.2z"/>',
    ],
    props
  );

export const Discord = (props?: IconProps): string =>
  createSvg(
    [
      '<circle cx="9" cy="12" r="1"/>',
      '<circle cx="15" cy="12" r="1"/>',
      '<path d="M7.5 7.2C8.7 6.48 10.2 6 12 6c1.8 0 3.3.48 4.5 1.2"/>',
      '<path d="M7 17c-1.5 0-2.4-1.2-2.8-2.7-.7-2.5-.4-4.9 1.2-7 1-1.3 2.3-2.3 4-2.8"/>',
      '<path d="M17 17c1.5 0 2.4-1.2 2.8-2.7.7-2.5.4-4.9-1.2-7-1-1.3-2.3-2.3-4-2.8"/>',
    ],
    props
  );

export const Youtube = (props?: IconProps): string =>
  createSvg(
    [
      '<path d="M2.5 17a24.12 24.12 0 0 1 0-10 2 2 0 0 1 1.4-1.4 49.56 49.56 0 0 1 16.2 0A2 2 0 0 1 21.5 7a24.12 24.12 0 0 1 0 10 2 2 0 0 1-1.4 1.4 49.55 49.55 0 0 1-16.2 0A2 2 0 0 1 2.5 17"/>',
      '<path d="m10 15 5-3-5-3z"/>',
    ],
    props
  );

export const icons = {
  Globe,
  Gamepad2,
  Zap,
  UserPlus,
  ArrowRight,
  Trophy,
  MousePointer,
  Timer,
  Users,
  Award,
  Menu,
  Volume2,
  VolumeX,
  Wind,
  ChevronLeft,
  ChevronRight,
  Mail,
  Send,
  LogIn,
  LogOut,
  Home,
  RotateCcw,
  Play,
  Copy,
  Github,
  Twitter,
  Discord,
  Youtube,
} as const;

export type IconName = keyof typeof icons;

export function getIcon(name: IconName, props?: IconProps): string {
  return icons[name](props);
}
