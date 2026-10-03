import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { useAllFlags } from "@flagpole/sdk/react";

export const XRay = createContext(false);

/**
 * Marks the part of the page a flag controls. It flashes when the flag's
 * variant changes, and x-ray mode outlines it with the flag's name.
 */
export function Flagged({ flag, children, className = "" }: { flag: string; children: ReactNode; className?: string }) {
  const result = useAllFlags()[flag];
  const xray = useContext(XRay);
  const variant = result?.variant ?? "";
  const last = useRef(variant);
  const [flash, setFlash] = useState(false);

  useEffect(() => {
    if (last.current === variant) return;
    last.current = variant;
    setFlash(true);
    const t = setTimeout(() => setFlash(false), 1800);
    return () => clearTimeout(t);
  }, [variant]);

  return (
    <div className={`flagged ${flash ? "flash" : ""} ${xray ? "xray" : ""} ${className}`}>
      <span className="flag-tag">
        {flag} · {variant || "?"}
      </span>
      {children}
    </div>
  );
}
