import React from "react";
import { APP_CONFIG } from "@/lib/constants";

export function Footer() {
  return (
    <footer className="w-full border-t border-slate-800/80 bg-slate-950 py-8 text-center text-xs text-slate-500">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 flex flex-col sm:flex-row items-center justify-between gap-4">
        <p>
          © {new Date().getFullYear()} {APP_CONFIG.name}. Built for accessibility.
        </p>
        <div className="flex items-center gap-4">
          <span className="inline-flex items-center gap-1.5 text-emerald-400">
            <span className="h-2 w-2 rounded-full bg-emerald-400 animate-pulse" />
            Backend Connected
          </span>
        </div>
      </div>
    </footer>
  );
}
