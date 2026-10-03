// International Code of Signals flags, one per environment. Production
// flies Papa: by tradition, "the vessel is about to sail".

type Flag = { letter: string; meaning: string; draw: React.ReactNode };

const Y = "#f2b705";
const B = "#1f4e9a";
const R = "#c8102e";
const W = "#ffffff";

const flags: Record<string, Flag> = {
  papa: { letter: "P", meaning: "about to sail", draw: <><rect width="30" height="20" fill={B} /><rect x="8" y="5.5" width="14" height="9" fill={W} /></> },
  delta: { letter: "D", meaning: "keep clear", draw: <><rect width="30" height="20" fill={Y} /><rect y="5" width="30" height="10" fill={B} /></> },
  sierra: { letter: "S", meaning: "engines going astern", draw: <><rect width="30" height="20" fill={W} /><rect x="10" y="5.5" width="10" height="9" fill={B} /></> },
  echo: { letter: "E", meaning: "altering course to starboard", draw: <><rect width="30" height="10" fill={B} /><rect y="10" width="30" height="10" fill={R} /></> },
  hotel: { letter: "H", meaning: "pilot on board", draw: <><rect width="15" height="20" fill={W} /><rect x="15" width="15" height="20" fill={R} /></> },
  kilo: { letter: "K", meaning: "I wish to communicate", draw: <><rect width="15" height="20" fill={Y} /><rect x="15" width="15" height="20" fill={B} /></> },
  golf: { letter: "G", meaning: "I require a pilot", draw: <>{[0, 1, 2, 3, 4, 5].map((i) => <rect key={i} x={i * 5} width="5" height="20" fill={i % 2 ? B : Y} />)}</> },
  foxtrot: { letter: "F", meaning: "I am disabled", draw: <><rect width="30" height="20" fill={W} /><path d="M15 2 L27 10 L15 18 L3 10Z" fill={R} /></> },
  uniform: { letter: "U", meaning: "standing into danger", draw: <><rect width="30" height="20" fill={W} /><rect width="15" height="10" fill={R} /><rect x="15" y="10" width="15" height="10" fill={R} /></> },
};

const byEnvironment: Record<string, string> = { production: "papa", prod: "papa", development: "delta", dev: "delta", staging: "sierra", stage: "sierra" };
const spares = ["echo", "hotel", "kilo", "golf", "foxtrot", "uniform"];

export function flagFor(envKey: string): Flag {
  const name = byEnvironment[envKey];
  if (name) return flags[name];
  let h = 0;
  for (const ch of envKey) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return flags[spares[h % spares.length]];
}

/** The environment's signal flag. */
export function SignalFlag({ env, size = 22, title }: { env: string; size?: number; title?: boolean }) {
  const f = flagFor(env);
  return (
    <svg width={size * 1.5} height={size} viewBox="0 0 30 20" role="img" aria-label={`${env} (${f.letter})`} className="shrink-0 rounded-[2px] shadow-[0_0_0_1px_rgb(16_24_40/0.25)]">
      {title !== false && <title>{`${env} · flag ${f.letter}, "${f.meaning}"`}</title>}
      {f.draw}
    </svg>
  );
}

/** A pennant on a mast: hoisted when on, lowered when off. */
export function Hoist({ env, on, rules = 0 }: { env: string; on: boolean; rules?: number }) {
  const f = flagFor(env);
  return (
    <span className="inline-flex items-end gap-1.5" title={`${env}: ${on ? "on" : "off"}${rules ? `, ${rules} rule${rules > 1 ? "s" : ""}` : ""}`}>
      <svg width="26" height="30" viewBox="0 0 26 30" aria-hidden="true">
        <rect x="2" y="1" width="1.6" height="28" rx="0.8" fill="currentColor" className="text-ink-2" />
        <circle cx="2.8" cy="1.6" r="1.4" fill="currentColor" className="text-ink-2" />
        <g
          style={{ transform: on ? "translateY(0)" : "translateY(16px)", transition: "transform 0.45s cubic-bezier(.2,.8,.2,1), opacity 0.3s" }}
          opacity={on ? 1 : 0.35}
        >
          <svg x="3.6" y="2" width="21" height="14" viewBox="0 0 30 20">{f.draw}</svg>
        </g>
      </svg>
      <span className={`mono text-[11px] leading-none ${on ? "text-go font-medium" : "text-muted"}`}>
        {on ? "on" : "off"}
        {on && rules > 0 && <span className="text-muted"> ·{rules}</span>}
      </span>
    </span>
  );
}
