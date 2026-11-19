"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { isActive, visibleNav } from "@/lib/admin/nav";
import type { Role } from "@/lib/admin/permissions";

/** Grouped navigation. Phase 2/3 screens are listed but disabled with their phase tag. */
export function SidebarNav({ role }: { role: Role }) {
  const pathname = usePathname() ?? "";
  return <NavList role={role} pathname={pathname} />;
}

export function NavList({ role, pathname }: { role: Role; pathname: string }) {
  return (
    <nav aria-label="Quản trị" className="flex flex-col gap-4">
      {visibleNav(role).map((g) => (
        <div key={g.title}>
          <div className="px-3 pb-1.5 text-[11px] font-bold tracking-wider text-white/50">{g.title}</div>
          <ul className="flex flex-col gap-0.5">
            {g.items.map((item) =>
              item.phase ? (
                <li key={item.href}>
                  <span aria-disabled="true" className="flex h-10 cursor-not-allowed items-center justify-between rounded-lg px-3 text-sm text-white/40">
                    {item.label}
                    <span className="rounded-full border border-white/20 px-1.5 text-[10px] font-bold">{item.phase}</span>
                  </span>
                </li>
              ) : (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={isActive(item.href, pathname) ? "page" : undefined}
                    className="flex h-10 items-center rounded-lg px-3 text-sm font-medium text-white/85 hover:bg-navy-2 aria-[current=page]:bg-navy-2 aria-[current=page]:font-bold aria-[current=page]:text-white"
                  >
                    {item.label}
                  </Link>
                </li>
              ),
            )}
          </ul>
        </div>
      ))}
    </nav>
  );
}
