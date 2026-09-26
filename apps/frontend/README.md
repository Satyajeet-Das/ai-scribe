# AI Exam Scribe Frontend

Production-grade Next.js App Router frontend for the AI Exam Scribe platform.

## Architecture & Folder Structure

The application follows an enterprise Next.js App Router structure with feature-sliced modularity:

```
apps/frontend/
├── public/                 # Static assets (favicons, SVG icons, images)
├── src/
│   ├── app/                # Next.js App Router routes & layouts
│   │   ├── (auth)/         # Route group for authentication
│   │   │   └── login/      # Sign-in / session authentication route
│   │   ├── (dashboard)/    # Route group for authenticated workspace
│   │   │   ├── exams/      # Exam listing & management
│   │   │   └── sessions/   # Exam session interface
│   │   ├── error.tsx       # Root error boundary
│   │   ├── favicon.ico     # Application icon
│   │   ├── globals.css     # Global CSS and Tailwind directives
│   │   ├── layout.tsx      # Root HTML layout & fonts
│   │   ├── loading.tsx     # Root loading fallback
│   │   ├── not-found.tsx   # 404 handler
│   │   └── page.tsx        # Landing / Home page
│   ├── components/         # Shared presentation components
│   │   ├── layout/         # Shell components (Header, Footer, Navigation)
│   │   └── ui/             # Reusable design tokens (Button, Card, Badge)
│   ├── features/           # Domain/feature-sliced modules
│   │   ├── auth/           # Authentication state & handlers
│   │   ├── exams/          # Exam catalog, questions, assignments
│   │   └── sessions/       # Exam sessions & interaction state
│   ├── hooks/              # Shared generic React hooks (e.g., accessibility)
│   ├── lib/                # Core utilities, API clients, constants
│   │   ├── api.ts          # Strongly typed fetch client
│   │   ├── constants.ts    # Application configuration & route endpoints
│   │   └── utils.ts        # Tailwind class merging (clsx / twMerge)
│   ├── providers/          # Context providers (Theme, Auth, Query)
│   ├── services/           # External API & service integrations
│   └── types/              # Domain TypeScript types & interfaces
│       ├── api.ts          # API response & error structures
│       ├── exam.ts         # Exam, question, assignment models
│       └── session.ts      # Exam session & answer status models
├── .env.example            # Environment variable template
├── next.config.ts          # Next.js configuration & API reverse proxy rewrites
├── package.json            # Dependencies & build scripts
├── postcss.config.mjs      # PostCSS & Tailwind configuration
├── tailwind.config.ts      # Tailwind CSS configuration
└── tsconfig.json           # TypeScript configuration with `@/*` path mapping
```

## Backend API Integration

Development API requests under `/api/v1/:path*` and `/status` are automatically proxied to the Go backend (`http://localhost:8080`) via `next.config.ts` rewrites to prevent CORS issues during local development.

## Scripts

```bash
# Start development server
bun dev # or npm run dev

# Build production bundle
bun run build # or npm run build

# Start production server
bun run start # or npm run start

# Run ESLint
bun run lint # or npm run lint
```
