import { useState, type ReactNode } from "react";
import { useFlag } from "@flagpole/sdk/react";
import { Art, money, products } from "./catalogue";
import { Flagged } from "./flagged";

export type Cart = Record<string, number>;

export const buyColours: Record<string, string> = { blue: "var(--buy-blue)", green: "var(--buy-green)", orange: "var(--buy-orange)" };

export function BuyButton({ children, onClick, className = "", type = "button", disabled }: { children: ReactNode; onClick?: () => void; className?: string; type?: "button" | "submit"; disabled?: boolean }) {
  const colour = useFlag("button-color", "blue");
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      style={{ background: buyColours[colour] ?? buyColours.blue }}
      className={`rounded-full px-5 py-2.5 text-sm font-semibold text-white shadow-sm transition hover:brightness-110 active:scale-[0.98] disabled:opacity-50 ${className}`}
    >
      {children}
    </button>
  );
}

export function totals(cart: Cart, threshold: number) {
  const subtotal = products.reduce((s, p) => s + p.price * (cart[p.id] ?? 0), 0);
  const shipping = subtotal === 0 || subtotal >= threshold ? 0 : 4.95;
  return { subtotal, shipping, total: subtotal + shipping };
}

function Lines({ cart, onQty }: { cart: Cart; onQty?: (id: string, n: number) => void }) {
  return (
    <ul className="divide-y divide-rule">
      {products.filter((p) => cart[p.id]).map((p) => (
        <li key={p.id} className="flex items-center gap-3 py-3">
          <span className="rounded-lg p-1" style={{ background: p.tint }}><Art product={p} size={44} /></span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold">{p.name}</span>
            <span className="text-xs text-muted">{money(p.price)} each</span>
          </span>
          {onQty ? (
            <span className="flex items-center gap-1 rounded-full border border-rule px-1 text-sm">
              <button className="px-2 py-0.5 text-muted hover:text-ink" onClick={() => onQty(p.id, cart[p.id] - 1)} aria-label={`One fewer ${p.name}`}>−</button>
              <span className="w-5 text-center tabular-nums">{cart[p.id]}</span>
              <button className="px-2 py-0.5 text-muted hover:text-ink" onClick={() => onQty(p.id, cart[p.id] + 1)} aria-label={`One more ${p.name}`}>+</button>
            </span>
          ) : (
            <span className="text-sm text-muted">× {cart[p.id]}</span>
          )}
        </li>
      ))}
    </ul>
  );
}

function Summary({ cart, threshold }: { cart: Cart; threshold: number }) {
  const t = totals(cart, threshold);
  return (
    <dl className="space-y-1 text-sm">
      <div className="flex justify-between"><dt className="text-ink-2">Subtotal</dt><dd className="tabular-nums">{money(t.subtotal)}</dd></div>
      <div className="flex justify-between"><dt className="text-ink-2">Shipping</dt><dd className="tabular-nums">{t.shipping ? money(t.shipping) : "Free"}</dd></div>
      <div className="flex justify-between border-t border-rule pt-2 text-base font-semibold"><dt>Total</dt><dd className="tabular-nums">{money(t.total)}</dd></div>
    </dl>
  );
}

function Field({ label, ...props }: { label: string } & React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-semibold text-ink-2">{label}</span>
      <input {...props} className="w-full rounded-lg border border-rule bg-white/70 px-3 py-2 text-sm focus:border-sea focus:outline-none" />
    </label>
  );
}

function Address() {
  return (
    <div className="space-y-3">
      <Field label="Name" defaultValue="Ada Lovelace" />
      <Field label="Email" type="email" defaultValue="ada@harbour.test" />
      <Field label="Address" defaultValue="4 Quay Street" />
      <div className="grid grid-cols-2 gap-3">
        <Field label="Town" defaultValue="Falmouth" />
        <Field label="Postcode" defaultValue="TR11 3HH" />
      </div>
      <p className="rounded-lg bg-sea/8 px-3 py-2 text-xs text-ink-2">Payment is on delivery. This is a demo: nothing is charged or shipped.</p>
    </div>
  );
}

/** The cart and checkout drawer. new-checkout picks one page or three steps. */
export function Checkout({ cart, threshold, onQty, onClose, onPlaced }: { cart: Cart; threshold: number; onQty: (id: string, n: number) => void; onClose: () => void; onPlaced: () => void }) {
  const onePage = useFlag("new-checkout", false);
  const [step, setStep] = useState(0);
  const [placed, setPlaced] = useState<string | null>(null);
  const empty = totals(cart, threshold).subtotal === 0;
  const place = () => {
    setPlaced(`HS-${1000 + Math.floor(Math.random() * 9000)}`);
    onPlaced();
  };

  return (
    <div className="fixed inset-0 z-30 flex justify-end bg-ink/30 backdrop-blur-[2px]" onClick={onClose}>
      <div className="slide-in flex h-full w-[min(440px,100vw)] flex-col bg-card shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <header className="flex items-center justify-between border-b border-rule px-6 py-4">
          <h2 className="display text-2xl font-semibold">{placed ? "Thank you" : "Your order"}</h2>
          <button onClick={onClose} className="text-muted hover:text-ink" aria-label="Close">✕</button>
        </header>
        <div className="flex-1 overflow-y-auto px-6 py-5">
          {placed ? (
            <div className="py-10 text-center">
              <div className="display text-5xl">⚓</div>
              <p className="display mt-4 text-xl">Order {placed} is placed.</p>
              <p className="mt-2 text-sm text-ink-2">It was a {onePage ? "one-page" : "three-step"} checkout, chosen by the new-checkout flag.</p>
            </div>
          ) : empty ? (
            <p className="py-10 text-center text-sm text-muted">Your cart is empty.</p>
          ) : (
            <Flagged flag="new-checkout">
              {onePage ? (
                <div className="space-y-6">
                  <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.14em] text-sea">
                    <span className="h-1.5 w-1.5 rounded-full bg-sea" /> One-page checkout
                  </div>
                  <Lines cart={cart} onQty={onQty} />
                  <Address />
                  <Summary cart={cart} threshold={threshold} />
                  <BuyButton onClick={place} className="w-full py-3">Place order</BuyButton>
                </div>
              ) : (
                <div className="space-y-6">
                  <ol className="flex items-center gap-2 text-xs font-semibold">
                    {["Cart", "Delivery", "Review"].map((s, i) => (
                      <li key={s} className="flex flex-1 items-center gap-2">
                        <span className={`flex h-6 w-6 items-center justify-center rounded-full ${i <= step ? "bg-ink text-white" : "border border-rule text-muted"}`}>{i + 1}</span>
                        <span className={i <= step ? "" : "text-muted"}>{s}</span>
                        {i < 2 && <span className="h-px flex-1 bg-rule" />}
                      </li>
                    ))}
                  </ol>
                  {step === 0 && <Lines cart={cart} onQty={onQty} />}
                  {step === 1 && <Address />}
                  {step === 2 && (
                    <>
                      <Lines cart={cart} />
                      <Summary cart={cart} threshold={threshold} />
                    </>
                  )}
                  <div className="flex gap-2">
                    {step > 0 && <button onClick={() => setStep(step - 1)} className="rounded-full border border-rule px-5 py-2.5 text-sm font-semibold">Back</button>}
                    {step < 2 ? (
                      <BuyButton onClick={() => setStep(step + 1)} className="flex-1">Continue</BuyButton>
                    ) : (
                      <BuyButton onClick={place} className="flex-1">Place order</BuyButton>
                    )}
                  </div>
                </div>
              )}
            </Flagged>
          )}
        </div>
      </div>
    </div>
  );
}
