import { useMemo, useState } from "react";
import { useFlag, useFlagpole } from "@flagpole/sdk/react";
import { Art, money, products, type Product } from "./catalogue";
import { BuyButton, Checkout, totals, type Cart } from "./checkout";
import { Flagged, XRay } from "./flagged";
import { Inspector } from "./inspector";
import { newVisitor, people } from "./people";

function remembered(key: string, fallback: string): string {
  try {
    return localStorage.getItem(key) ?? fallback;
  } catch {
    return fallback;
  }
}

function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* private mode */
  }
}

export function App() {
  const client = useFlagpole();
  const [person, setPerson] = useState(() => remembered("harbour-person", "ada"));
  const [xray, setXray] = useState(false);
  const [cart, setCart] = useState<Cart>({});
  const [open, setOpen] = useState(false);
  const [bump, setBump] = useState(0);
  const [query, setQuery] = useState("");
  const threshold = useFlag("free-shipping-threshold", 50);

  const choose = (id: string) => {
    setPerson(id);
    remember("harbour-person", id);
    void client.identify(people.find((p) => p.id === id)!.context());
  };
  const add = (p: Product) => {
    setCart((c) => ({ ...c, [p.id]: (c[p.id] ?? 0) + 1 }));
    setBump((b) => b + 1);
  };
  const setQty = (id: string, n: number) => setCart((c) => ({ ...c, [id]: Math.max(0, n) }));
  const count = Object.values(cart).reduce((a, b) => a + b, 0);
  const { subtotal } = totals(cart, threshold);
  const shown = useMemo(() => products.filter((p) => !query || `${p.name} ${p.detail}`.toLowerCase().includes(query.toLowerCase())), [query]);

  return (
    <XRay.Provider value={xray}>
      <MaintenanceBanner />
      <Flagged flag="free-shipping-threshold" className="rounded-none">
        <div className="bg-sea px-4 py-2 text-center text-[13px] text-white">
          {subtotal > 0 && subtotal < threshold ? (
            <>You're <strong>{money(threshold - subtotal)}</strong> away from free shipping</>
          ) : subtotal >= threshold ? (
            <>Your order ships <strong>free</strong></>
          ) : (
            <>Free UK shipping on orders over <strong>{money(threshold)}</strong></>
          )}
        </div>
      </Flagged>

      <header className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-8 gap-y-4 px-5 pt-7 pb-5">
        <a href="/" className="flex items-center gap-3">
          <svg viewBox="0 0 32 32" className="h-10 w-10" aria-hidden="true">
            <circle cx="16" cy="16" r="15" fill="#1d2a2f" />
            <path d="M16 7v17M10 19.5a6 6 0 0 0 12 0M12.5 10.5h7" stroke="#d9a441" strokeWidth="2.2" fill="none" strokeLinecap="round" />
          </svg>
          <span className="leading-none">
            <span className="display block text-2xl font-semibold">Harbour Supply Co.</span>
            <span className="text-[11px] uppercase tracking-[0.24em] text-muted">Chandlers since 1897</span>
          </span>
        </a>
        <div className="order-3 w-full sm:order-none sm:w-auto sm:flex-1">
          <Search query={query} onQuery={setQuery} />
        </div>
        <button onClick={() => setOpen(true)} className="ml-auto flex items-center gap-2 rounded-full border border-ink/15 bg-card px-4 py-2 text-sm font-semibold hover:border-ink/40 sm:ml-0">
          Cart
          <span key={bump} className={`flex h-6 min-w-6 items-center justify-center rounded-full bg-ink px-1.5 text-xs text-white ${bump ? "bump" : ""}`}>{count}</span>
        </button>
      </header>

      <main className="mx-auto max-w-6xl px-5 pb-40">
        <section className="relative overflow-hidden rounded-3xl bg-ink px-8 py-12 text-[#f3eee4] sm:px-12 sm:py-16">
          <svg viewBox="0 0 200 200" className="absolute -top-10 -right-10 h-80 w-80 opacity-[0.12]" aria-hidden="true">
            <circle cx="100" cy="100" r="90" fill="none" stroke="#d9a441" strokeWidth="2" />
            <circle cx="100" cy="100" r="70" fill="none" stroke="#d9a441" strokeWidth="1" />
            {Array.from({ length: 16 }, (_, i) => (
              <path key={i} d={`M100 100 L${100 + (i % 2 ? 50 : 90) * Math.sin((i * Math.PI) / 8)} ${100 - (i % 2 ? 50 : 90) * Math.cos((i * Math.PI) / 8)}`} stroke="#d9a441" strokeWidth={i % 4 === 0 ? 3 : 1} />
            ))}
          </svg>
          <p className="text-xs font-semibold uppercase tracking-[0.3em] text-[#d9a441]">Autumn catalogue</p>
          <h1 className="display mt-4 max-w-xl text-5xl leading-[1.02] font-semibold sm:text-6xl">
            Gear for the <em className="font-normal">long haul.</em>
          </h1>
          <p className="mt-5 max-w-md text-[15px] leading-relaxed text-[#f3eee4]/75">
            Rope, lamps and oilskins for working boats, chosen by people who still go out when the forecast says not to.
          </p>
        </section>

        <div className="mt-12 mb-5 flex items-baseline justify-between">
          <h2 className="display text-3xl font-semibold">{query ? `Matching “${query}”` : "In the shop"}</h2>
          <span className="text-sm text-muted">{shown.length} of {products.length}</span>
        </div>
        <Flagged flag="button-color">
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {shown.map((p) => (
              <article key={p.id} className="group overflow-hidden rounded-2xl border border-ink/10 bg-card transition hover:-translate-y-0.5 hover:shadow-[0_18px_40px_-24px_rgb(29_42_47/0.45)]">
                <div className="flex justify-center py-6" style={{ background: p.tint }}>
                  <span className="transition group-hover:scale-105"><Art product={p} size={140} /></span>
                </div>
                <div className="flex items-end justify-between gap-3 p-5">
                  <div>
                    <h3 className="display text-xl leading-tight font-semibold">{p.name}</h3>
                    <p className="mt-0.5 text-sm text-muted">{p.detail}</p>
                    <p className="mt-2 text-lg font-semibold tabular-nums">{money(p.price)}</p>
                  </div>
                  <BuyButton onClick={() => add(p)}>Add to cart</BuyButton>
                </div>
              </article>
            ))}
          </div>
        </Flagged>
        {shown.length === 0 && <p className="py-10 text-center text-muted">Nothing matches. Try “rope” or “brass”.</p>}

        <footer className="mt-20 border-t border-rule pt-6 text-sm text-muted">
          Harbour Supply Co. is a demo shop for Flagpole: every highlighted part of this page is controlled by a flag in web-app's development environment. Nothing here is for sale.
        </footer>
      </main>

      {open && <Checkout cart={cart} threshold={threshold} onQty={setQty} onClose={() => setOpen(false)} onPlaced={() => setCart({})} />}
      <Inspector
        person={person}
        onPerson={choose}
        onNewVisitor={() => {
          newVisitor();
          choose("visitor");
        }}
        xray={xray}
        onXray={setXray}
      />
    </XRay.Provider>
  );
}

function MaintenanceBanner() {
  const on = useFlag("maintenance-banner", false);
  if (!on) return null;
  return (
    <Flagged flag="maintenance-banner" className="rounded-none">
      <div className="hazard p-1.5">
        <div className="bg-ink px-4 py-2 text-center text-[13px] text-[#f3eee4]">
          <strong className="text-[#e3b23c]">Maintenance tonight, 22:00–23:00.</strong> You can browse, but orders placed then ship on Monday.
        </div>
      </div>
    </Flagged>
  );
}

/** search-v2 searches as you type and suggests products; the old search waits for Enter. */
function Search({ query, onQuery }: { query: string; onQuery: (q: string) => void }) {
  const v2 = useFlag("search-v2", false);
  const [text, setText] = useState(query);
  const [focused, setFocused] = useState(false);
  const suggestions = v2 && text ? products.filter((p) => `${p.name} ${p.detail}`.toLowerCase().includes(text.toLowerCase())).slice(0, 4) : [];

  return (
    <Flagged flag="search-v2">
      <form
        className="relative"
        onSubmit={(e) => {
          e.preventDefault();
          onQuery(text.trim());
        }}
      >
        <div className="flex items-center rounded-full border border-ink/15 bg-card pr-1.5 focus-within:border-sea">
          <svg viewBox="0 0 20 20" className="ml-4 h-4 w-4 shrink-0 text-muted" aria-hidden="true"><circle cx="8.5" cy="8.5" r="5.5" fill="none" stroke="currentColor" strokeWidth="2" /><path d="M13 13l4 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" /></svg>
          <input
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              if (v2) onQuery(e.target.value.trim());
              else if (!e.target.value) onQuery("");
            }}
            onFocus={() => setFocused(true)}
            onBlur={() => setTimeout(() => setFocused(false), 150)}
            placeholder={v2 ? "Search as you type…" : "Search the catalogue"}
            className="w-full bg-transparent px-3 py-2.5 text-sm focus:outline-none"
          />
          {v2 ? (
            <span className="rounded-full bg-sea/12 px-2.5 py-1 text-[11px] font-semibold text-sea">Instant</span>
          ) : (
            <button type="submit" className="rounded-full bg-ink px-4 py-1.5 text-xs font-semibold text-white">Search</button>
          )}
        </div>
        {focused && suggestions.length > 0 && (
          <ul className="absolute top-full right-0 left-0 z-20 mt-2 overflow-hidden rounded-2xl border border-ink/10 bg-card shadow-xl">
            {suggestions.map((p) => (
              <li key={p.id}>
                <button type="button" onClick={() => { setText(p.name); onQuery(p.name); }} className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-paper">
                  <span className="rounded-md" style={{ background: p.tint }}><Art product={p} size={36} /></span>
                  <span className="flex-1 text-sm font-semibold">{p.name}</span>
                  <span className="text-sm text-muted">{money(p.price)}</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </form>
    </Flagged>
  );
}
