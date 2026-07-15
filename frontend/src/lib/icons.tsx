import type { CSSProperties, ReactElement } from "react";

/* Icon set ported from prototype/assets/app.js (consistent 1.8 stroke,
   currentColor) plus a few common extras the screens reference. */

const stroke = {
  stroke: "currentColor",
  strokeWidth: 1.8,
  fill: "none",
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

const P: Record<string, ReactElement> = {
  grid: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1.5" {...stroke} />
      <rect x="14" y="3" width="7" height="7" rx="1.5" {...stroke} />
      <rect x="3" y="14" width="7" height="7" rx="1.5" {...stroke} />
      <rect x="14" y="14" width="7" height="7" rx="1.5" {...stroke} />
    </>
  ),
  board: (
    <>
      <rect x="3" y="4" width="5" height="16" rx="1.5" {...stroke} />
      <rect x="10" y="4" width="5" height="11" rx="1.5" {...stroke} />
      <rect x="17" y="4" width="4" height="14" rx="1.5" {...stroke} />
    </>
  ),
  bot: (
    <>
      <rect x="4" y="8" width="16" height="11" rx="3" {...stroke} />
      <path d="M12 8V4M9 14h.01M15 14h.01M9 19v2M15 19v2" {...stroke} />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" {...stroke} />
      <path d="M12 7v5l3 2" {...stroke} />
    </>
  ),
  gear: (
    <>
      <circle cx="12" cy="12" r="3.2" {...stroke} />
      <path
        d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"
        {...stroke}
      />
    </>
  ),
  bell: (
    <path
      d="M6 8a6 6 0 1 1 12 0c0 5 2 6 2 6H4s2-1 2-6ZM10 20a2 2 0 0 0 4 0"
      {...stroke}
    />
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="7" {...stroke} />
      <path d="m20 20-3.2-3.2" {...stroke} />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" {...stroke} />,
  pause: (
    <>
      <rect x="6" y="5" width="4" height="14" rx="1" {...stroke} />
      <rect x="14" y="5" width="4" height="14" rx="1" {...stroke} />
    </>
  ),
  play: <path d="M7 4v16l13-8z" {...stroke} />,
  check: <path d="M20 6 9 17l-5-5" {...stroke} />,
  send: <path d="M22 2 11 13M22 2l-7 20-4-9-9-4 20-7Z" {...stroke} />,
  stop: <rect x="6" y="6" width="12" height="12" rx="2" {...stroke} />,
  code: <path d="m8 6-6 6 6 6M16 6l6 6-6 6" {...stroke} />,
  file: (
    <>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" {...stroke} />
      <path d="M14 3v5h5" {...stroke} />
    </>
  ),
  // ── extras referenced by the screens ──
  close: <path d="M6 6l12 12M18 6 6 18" {...stroke} />,
  chevronDown: <path d="m6 9 6 6 6-6" {...stroke} />,
  chevronRight: <path d="m9 6 6 6-6 6" {...stroke} />,
  filter: <path d="M3 5h18l-7 8v6l-4-2v-4z" {...stroke} />,
  more: (
    <>
      <circle cx="5" cy="12" r="1.4" fill="currentColor" stroke="none" />
      <circle cx="12" cy="12" r="1.4" fill="currentColor" stroke="none" />
      <circle cx="19" cy="12" r="1.4" fill="currentColor" stroke="none" />
    </>
  ),
  calendar: (
    <>
      <rect x="3" y="5" width="18" height="16" rx="2" {...stroke} />
      <path d="M3 9h18M8 3v4M16 3v4" {...stroke} />
    </>
  ),
  download: <path d="M12 3v12m-5-5 5 5 5-5M5 21h14" {...stroke} />,
  github: (
    <path
      d="M9 19c-4 1.5-4-2.5-6-3m12 5v-3.5c0-1 .1-1.4-.5-2 2.8-.3 5.5-1.4 5.5-6a4.6 4.6 0 0 0-1.3-3.2 4.3 4.3 0 0 0-.1-3.2s-1-.3-3.4 1.3a11.6 11.6 0 0 0-6 0C6.3 2.3 5.3 2.6 5.3 2.6a4.3 4.3 0 0 0-.1 3.2A4.6 4.6 0 0 0 3.9 9c0 4.6 2.7 5.7 5.5 6-.6.6-.6 1.2-.5 2V21"
      {...stroke}
    />
  ),
  comment: <path d="M21 12a8 8 0 0 1-11.5 7.2L4 21l1.8-4.5A8 8 0 1 1 21 12Z" {...stroke} />,
  link: (
    <>
      <path d="M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-1 1" {...stroke} />
      <path d="M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l1-1" {...stroke} />
    </>
  ),
  alert: (
    <>
      <path d="M12 3 2 20h20z" {...stroke} />
      <path d="M12 10v4M12 17h.01" {...stroke} />
    </>
  ),
  git: (
    <>
      <circle cx="6" cy="6" r="2.4" {...stroke} />
      <circle cx="6" cy="18" r="2.4" {...stroke} />
      <circle cx="18" cy="12" r="2.4" {...stroke} />
      <path d="M6 8.4v7.2M8.4 6h6.6a3 3 0 0 1 3 3v.2" {...stroke} />
    </>
  ),
  paperclip: (
    <path
      d="M20 11.5 12 19.5a4.5 4.5 0 0 1-6.4-6.4l8-8a3 3 0 0 1 4.3 4.3l-8 8a1.5 1.5 0 0 1-2.2-2.2l7.3-7.3"
      {...stroke}
    />
  ),
  sparkle: <path d="M12 3v6M12 15v6M3 12h6M15 12h6M6 6l3 3M15 15l3 3M18 6l-3 3M9 15l-3 3" {...stroke} />,
};

export type IconName = keyof typeof P;

export function Icon({
  name,
  size = 18,
  style,
  className,
}: {
  name: IconName;
  size?: number;
  style?: CSSProperties;
  className?: string;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      className={className}
      style={{ display: "block", ...style }}
      aria-hidden="true"
      focusable="false"
    >
      {P[name]}
    </svg>
  );
}
