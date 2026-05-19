// UnderdogBadge is the small "Underdog" annotation shown on the underdog team
// of a match card — the side named by Match.underdog. It pairs with the orange
// side ring (see sideRingClass). The backend only sets Match.underdog on locked
// matches, so this never appears while predictions can still be changed.
export function UnderdogBadge() {
  return (
    <span className="shrink-0 rounded bg-orange-500 px-1.5 py-0.5 text-[10px] font-bold uppercase leading-none tracking-wide text-white">
      Underdog
    </span>
  );
}
