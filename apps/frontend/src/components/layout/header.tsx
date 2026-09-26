import React from "react";
import Link from "next/link";
import { APP_CONFIG } from "@/lib/constants";

export function Header() {
  return (
    <header className="sticky top-0 z-40 w-full border-b border-slate-800/80 bg-slate-950/80 backdrop-blur-md">
      <div className="max-w-7xl mx-auto flex h-16 items-center justify-between px-4 sm:px-6 lg:px-8">
        <div className="flex items-center gap-6">
          <Link href="/" className="flex items-center gap-2">
            <div className="h-8 w-8 rounded-lg bg-gradient-to-tr from-sky-500 to-indigo-600 flex items-center justify-center text-white font-bold text-sm">
              AS
            </div>
            <span className="font-bold text-lg text-white tracking-tight">{APP_CONFIG.name}</span>
          </Link>
          <nav className="hidden md:flex items-center gap-4 text-sm font-medium text-slate-400">
            <Link href="/exams" className="hover:text-white transition-colors">
              Exams
            </Link>
            <a
              href={`${APP_CONFIG.apiUrl}/docs`}
              target="_blank"
              rel="noreferrer"
              className="hover:text-white transition-colors"
            >
              API Docs
            </a>
          </nav>
        </div>

        <div className="flex items-center gap-3">
          <Link
            href="/login"
            className="text-xs font-semibold px-4 py-2 rounded-lg bg-sky-500 hover:bg-sky-400 text-white transition-colors"
          >
            Sign In
          </Link>
        </div>
      </div>
    </header>
  );
}
