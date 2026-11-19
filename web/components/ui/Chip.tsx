type Tone = "ok" | "warn" | "bad" | "info" | "navy";

const tones: Record<Tone, string> = {
  ok: "bg-ok-soft text-[#2f4f3e]",
  warn: "bg-gold-soft text-[#5c4400]",
  bad: "bg-bad-soft text-[#7a2a1e]",
  info: "bg-subtle text-muted",
  navy: "bg-navy text-white",
};

/** Status label. Always has text, never color alone. */
export function Chip({ tone = "info", children }: { tone?: Tone; children: React.ReactNode }) {
  return <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-bold ${tones[tone]}`}>{children}</span>;
}
