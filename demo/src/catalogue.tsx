import type { ReactNode } from "react";

export interface Product {
  id: string;
  name: string;
  detail: string;
  price: number;
  art: ReactNode;
  tint: string;
}

const ink = "#1d2a2f";
const brass = "#c9973f";
const sea = "#2f5d62";
const rust = "#b4532a";
const sail = "#efe6d4";

export const products: Product[] = [
  {
    id: "rope",
    name: "Three-strand manila",
    detail: "30 m coil, 12 mm",
    price: 34,
    tint: "#e9dcc2",
    art: (
      <g fill="none" strokeLinecap="round">
        {[34, 27, 20, 13].map((r, i) => (
          <circle key={r} cx="60" cy="62" r={r} stroke={i % 2 ? "#b58b4c" : "#9a7036"} strokeWidth="7" />
        ))}
        <circle cx="60" cy="62" r="6" fill="#9a7036" />
        <path d="M94 62 C104 62 108 80 98 92 C92 99 84 100 80 104" stroke="#9a7036" strokeWidth="7" />
        {[0, 1, 2, 3, 4, 5, 6, 7].map((i) => (
          <path key={i} d={`M${60 + 34 * Math.cos(i * 0.785)} ${62 + 34 * Math.sin(i * 0.785)} l3 3`} stroke="#7a5626" strokeWidth="2" />
        ))}
      </g>
    ),
  },
  {
    id: "lantern",
    name: "Brass storm lantern",
    detail: "Paraffin, 25 cm",
    price: 58,
    tint: "#ece0c4",
    art: (
      <g>
        <path d="M60 10 a10 10 0 0 1 10 10" fill="none" stroke={ink} strokeWidth="3" />
        <path d="M44 26 h32 l6 10 h-44z" fill={brass} />
        <rect x="42" y="36" width="36" height="46" rx="4" fill="#f7e7b0" stroke={brass} strokeWidth="3" />
        <path d="M60 48 c6 8 6 16 0 22 c-6 -6 -6 -14 0 -22z" fill={rust} />
        <path d="M60 56 c3 4 3 9 0 12 c-3 -3 -3 -8 0 -12z" fill="#f2b33d" />
        {[47, 73].map((x) => <rect key={x} x={x - 1.5} y="36" width="3" height="46" fill={brass} />)}
        <path d="M38 82 h44 l-4 12 h-36z" fill={brass} />
        <rect x="36" y="94" width="48" height="6" rx="2" fill={ink} />
      </g>
    ),
  },
  {
    id: "compass",
    name: "Hand bearing compass",
    detail: "Liquid filled, lit card",
    price: 42,
    tint: "#d9e3df",
    art: (
      <g>
        <circle cx="60" cy="60" r="40" fill={ink} />
        <circle cx="60" cy="60" r="33" fill={sail} />
        {Array.from({ length: 36 }, (_, i) => (
          <path key={i} d={`M60 ${i % 9 === 0 ? 30 : 29} V${i % 9 === 0 ? 37 : 33}`} stroke={ink} strokeWidth={i % 9 === 0 ? 2 : 1} transform={`rotate(${i * 10} 60 60)`} />
        ))}
        <path d="M60 34 L66 60 L60 86 L54 60z" fill={sea} />
        <path d="M60 34 L66 60 H54z" fill={rust} />
        <circle cx="60" cy="60" r="3.5" fill={brass} />
        <text x="60" y="47" textAnchor="middle" fontSize="8" fontWeight="700" fill={ink} fontFamily="serif">N</text>
      </g>
    ),
  },
  {
    id: "smock",
    name: "Oilskin smock",
    detail: "Waxed cotton, yellow",
    price: 89,
    tint: "#efe0b4",
    art: (
      <g>
        <path d="M44 22 L60 30 L76 22 L98 34 L92 54 L82 50 L82 100 H38 V50 L28 54 L22 34z" fill="#e3b23c" stroke={ink} strokeWidth="2.5" strokeLinejoin="round" />
        <path d="M50 24 Q60 40 70 24" fill="#c99520" stroke={ink} strokeWidth="2.5" />
        <path d="M60 32 V64" stroke={ink} strokeWidth="2" />
        <rect x="46" y="70" width="28" height="14" rx="2" fill="#c99520" stroke={ink} strokeWidth="2" />
        {[40, 50, 60].map((y) => <circle key={y} cx="64" cy={y} r="1.8" fill={ink} />)}
      </g>
    ),
  },
  {
    id: "horn",
    name: "Hand-pump fog horn",
    detail: "Audible to 1 nautical mile",
    price: 24,
    tint: "#e6d6cc",
    art: (
      <g>
        <rect x="22" y="52" width="40" height="18" rx="9" fill={rust} stroke={ink} strokeWidth="2.5" />
        <path d="M62 56 L96 36 V86 L62 66z" fill={brass} stroke={ink} strokeWidth="2.5" strokeLinejoin="round" />
        <rect x="28" y="38" width="6" height="14" fill={ink} />
        <rect x="20" y="32" width="22" height="7" rx="3" fill={ink} />
        {[0, 1, 2].map((i) => (
          <path key={i} d={`M${102 + i * 6} ${48 - i * 4} q6 13 0 26`} stroke={sea} strokeWidth="2.5" fill="none" opacity={1 - i * 0.28} transform={`translate(0 ${i * 4})`} />
        ))}
      </g>
    ),
  },
  {
    id: "ditty",
    name: "Canvas ditty bag",
    detail: "Hand-roped, 30 × 45 cm",
    price: 19,
    tint: "#e4e0d2",
    art: (
      <g>
        <path d="M38 34 H82 L88 98 Q60 106 32 98z" fill={sail} stroke={ink} strokeWidth="2.5" strokeLinejoin="round" />
        <path d="M38 34 Q60 44 82 34" fill="none" stroke={ink} strokeWidth="2.5" />
        <path d="M36 54 Q60 62 85 54" fill="none" stroke={sea} strokeWidth="5" />
        <path d="M44 34 Q60 6 76 34" fill="none" stroke="#9a7036" strokeWidth="4" />
        {[46, 60, 74].map((x) => <circle key={x} cx={x} cy="40" r="2.6" fill={brass} stroke={ink} strokeWidth="1" />)}
      </g>
    ),
  },
];

export const money = (n: number) => `£${n.toFixed(n % 1 ? 2 : 0)}`;

export function Art({ product, size = 120 }: { product: Product; size?: number }) {
  return (
    <svg viewBox="0 0 120 120" width={size} height={size} aria-hidden="true">
      {product.art}
    </svg>
  );
}
