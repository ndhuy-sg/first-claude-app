import type { ButtonHTMLAttributes } from "react";

type Variant = "primary" | "secondary" | "danger" | "ghost" | "navy";
type Size = "md" | "lg" | "xl";

const variants: Record<Variant, string> = {
  primary: "bg-gold text-navy font-bold hover:brightness-95 disabled:bg-subtle disabled:text-muted",
  secondary: "bg-white text-ink border border-line-strong font-semibold hover:bg-subtle disabled:text-muted",
  danger: "bg-white text-bad border border-bad/40 font-semibold hover:bg-bad-soft disabled:text-muted",
  ghost: "bg-transparent text-ink font-semibold hover:bg-subtle",
  navy: "bg-navy text-white font-semibold hover:bg-navy-2 disabled:bg-subtle disabled:text-muted",
};

// Touch targets: md 44px (admin), lg 56px (POS), xl 64px (POS primary actions)
const sizes: Record<Size, string> = {
  md: "h-11 px-4 text-[15px] rounded-[10px]",
  lg: "h-14 px-5 text-base rounded-xl",
  xl: "h-16 px-6 text-lg rounded-[14px]",
};

export function Button({
  variant = "secondary",
  size = "md",
  className = "",
  type = "button",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size }) {
  return (
    <button
      type={type}
      className={`inline-flex items-center justify-center gap-2 transition disabled:cursor-not-allowed ${variants[variant]} ${sizes[size]} ${className}`}
      {...props}
    />
  );
}
