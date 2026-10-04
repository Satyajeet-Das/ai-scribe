import React from "react";
import { APP_CONFIG, DESIGN_TOKENS } from "@/lib/constants";

export function Footer() {
  return (
    <footer className="w-full border-t border-border bg-background/80 py-6 text-xs text-muted-foreground">
      <div
        className={`${DESIGN_TOKENS.layout.container} flex flex-col sm:flex-row items-center justify-between gap-4`}
      >
        <p suppressHydrationWarning>
          &copy; {new Date().getFullYear()} {APP_CONFIG.name} · Accessible Voice-First Examination
          Platform
        </p>
        <div className="flex items-center gap-4">
          <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="size-2 rounded-full bg-emerald-500 animate-pulse" />
            Platform Active
          </span>
        </div>
      </div>
    </footer>
  );
}
export default Footer;
