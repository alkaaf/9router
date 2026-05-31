# Analysis 4: Usage Page — UI Structure & CSS

## Overview

Analisis struktur tampilan dan styling halaman Usage.

## File Structure

```
src/app/(dashboard)/dashboard/usage/
├── page.js
└── components/
    ├── OverviewCards.js
    ├── ProviderTopology.js
    ├── RequestDetailsTab.js
    ├── UsageChart.js
    ├── UsageTable.js
    └── ProviderLimits/
        ├── index.js
        ├── ProviderLimitCard.js
        ├── QuotaProgressBar.js
        ├── QuotaTable.js
        └── utils.js

src/shared/components/
├── Card.js              — Reusable card shell
├── Button.js            — Button variants (primary/secondary/outline/ghost/danger)
├── SegmentedControl.js  — Tab-like toggle buttons
├── Badge.js             — Status badges
├── Drawer.js            — Side panel drawer
├── Pagination.js        — Table pagination
├── Loading.js           — Spinner, Skeleton, CardSkeleton
├── UsageStats.js        — Main usage dashboard orchestrator
├── RequestLogger.js     — Live log stream
└── layouts/
    └── DashboardLayout.js — Shell layout (sidebar + main content)

src/app/globals.css        — Full design system (497 lines)
```

## Design System (`globals.css`)

### CSS Variables (Light/Dark)

| Variable | Light | Dark | Digunakan Untuk |
|----------|-------|------|-----------------|
| `--color-brand-500` | `#E56A4A` | `#E56A4A` | Primary brand color |
| `--color-primary` | brand-500 | `#E56A4A` | Primary actions |
| `--color-bg` | `#FDFAF6` | `#1a1a1a` | Page background |
| `--color-surface` | `#ffffff` | `#262626` | Card backgrounds |
| `--color-surface-2` | `#f4f4f5` | `#303030` | Elevated surfaces |
| `--color-border` | `#e5e7eb` | `#333333` | Borders |
| `--color-text-main` | `#0a0a0a` | `#ededed` | Primary text |
| `--color-text-muted` | `#6B7280` | `#9ca3af` | Secondary text |
| `--color-danger` | `#cf222e` | `#ef4444` | Error states |
| `--color-success` | `#10B981` | `#22c55e` | Success states |
| `--color-warning` | `#F59E0B` | `#fbbf24` | Warning/cost |

### Utility Classes

| Class | Fungsi |
|-------|--------|
| `.card-soft` | `box-shadow: var(--shadow-soft); border-radius: var(--radius-brand-lg)` |
| `.card-elev` | Elevated shadow + rounded corners |
| `.bg-vibrancy` | `backdrop-filter: blur(20px); background: rgba(255,255,255,0.72)` |
| `.animate-spin` | Spinning animation |
| `.animate-pulse` | Pulse animation |
| `.animate-border-glow` | Border glow animation |
| `.shadow-warm` | Warm orange glow shadow |

### Reusable Component Patterns

| Component | Pattern | Contoh Usage |
|-----------|---------|-------------|
| `Card` | `bg-surface border border-border-subtle rounded-[14px] shadow-soft` | Wraps every section |
| `Button` | Variants: `primary/secondary/outline/ghost/danger/success` | Aksi user |
| `SegmentedControl` | `inline-flex p-1 rounded-[10px] bg-surface-2` | Tab switching |
| `Badge` | `primary` + `neutral` variants, `sm` size | Status indicators |
| `Drawer` | Side panel, configurable width | Detail drawer di RequestDetailsTab |

## Layout Shell

### DashboardLayout

**File**: `src/shared/components/layouts/DashboardLayout.js`

```
┌──────────┬────────────────────────────────────┐
│ SIDEBAR  │  HEADER (top bar + hamburger)      │
│ w-72     ├────────────────────────────────────┤
│          │                                    │
│ Endpoint │  MAIN CONTENT                      │
│ Providers│  p-6 lg:p-10                       │
│ Combos   │  max-w-7xl mx-auto                 │
│ > Usage  │                                    │
│ Quota    │  [page content here]              │
│ MITM     │                                    │
│ CLI Tools│                                    │
│ ...      │                                    │
│          │                                    │
│ Shutdown │                                    │
└──────────┴────────────────────────────────────┘
```

- Desktop: sidebar always visible (`hidden lg:flex`)
- Mobile: off-canvas drawer (slide from left, `-translate-x-full` → `translate-x-0`)
- Background: `landing-grid` — faint accent grid lines (pointer-events-none)

### Sidebar Navigation

**File**: `src/shared/components/Sidebar.js`

**Main nav items** (line 19-28):
```js
const navItems = [
  { href: "/dashboard/endpoint",       label: "Endpoint",       icon: "api" },
  { href: "/dashboard/providers",      label: "Providers",      icon: "dns" },
  { href: "/dashboard/combos",         label: "Combos",         icon: "layers" },
  { href: "/dashboard/usage",          label: "Usage",          icon: "bar_chart" },
  { href: "/dashboard/quota",          label: "Quota Tracker",  icon: "data_usage" },
  { href: "/dashboard/mitm",           label: "MITM",           icon: "security" },
  { href: "/dashboard/cli-tools",      label: "CLI Tools",      icon: "terminal" },
];
```

**Active state** (line 70-75):
- Active: `bg-primary/10 text-primary` (brand tint bg + primary color)
- Inactive: `text-text-muted hover:bg-surface-2 hover:text-text-main`
- Icon: `fill-1` (filled) when active, outline when inactive

**Media Providers** — accordion submenu (line 42, 203-250):
- Accordion trigger: `perm_media` icon + "Media Providers" label + `expand_more` chevron
- Expand content: 4 visible kinds (embedding, image, tts, stt) + "Web Fetch & Search"

## Visual Structure Usage Page

```
┌─────────────────────────────────────────────────────────────┐
│  [Overview] [Details] [Logs]              [Today][24h][7D]… │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐       │
│  │ Total    │ │ Input    │ │ Output   │ │ Cost     │       │
│  │ Requests │ │ Tokens   │ │ Tokens   │ │ Est.     │  ← 4  │
│  │   1,234  │ │  56,789  │ │  23,456  │ │ $12.34   │    KPI │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘       │
├──────────────────────────────┬──────────────────────────────┤
│                              │  Recent Requests             │
│   Provider Topology (React   │  ┌─────────────────────────┐ │
│   Flow graph — nodes for     │  │ claude-sonnet-4  ✓ 2m ago│ │
│   each provider, hub at      │  │ gpt-4o           ✓ 5m ago│ │
│   center)                    │  │ gemini-pro       ✓ 8m ago│ │
│                              │  └─────────────────────────┘ │
├──────────────────────────────┴──────────────────────────────┤
│  Usage Chart (Area — recharts)                               │
│  [Tokens] [Cost] toggle                                      │
│  ▲  ▲  ▲  ▲                                                 │
│  ▲  ▲  ▲  ▲  ▲                                              │
│  ▲  ▲  ▲  ▲  ▲                                              │
│  ▲  ▲  ▲  ▲                                                 │
├─────────────────────────────────────────────────────────────┤
│  Usage Table                                                │
│  [Group by: Model ▼]              [Costs] [Tokens]          │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ ▼ claude-sonnet-4-6     1,234 reqs  $45.67         │   │
│  │   └─ cc (primary)       1,200 reqs  $44.20         │   │
│  │   └─ api (fallback)       34 reqs    $1.47         │   │
│  │ ▼ gpt-4o                   567 reqs  $12.34         │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Pattern untuk Halaman Baru

Untuk membuat halaman "Usage per User" yang menyerupai halaman Usage:

1. **Buat page component** di `src/app/(dashboard)/dashboard/usage-per-key/[keyId]/page.js`
2. **Gunakan Card** untuk section containers
3. **Gunakan SegmentedControl** untuk tab switching
4. **Gunakan same CSS utilities** (`bg-surface`, `border-border-subtle`, `rounded-[14px]`, dll)
5. **Fetch dari API baru** — `GET /api/usage/per-key/[keyId]?period=X`
6. **Reuse components** — `UsageChart`, `UsageTable`, `OverviewCards` bisa di-reuse dengan props berbeda

### File Terkait

| File | Peran |
|------|-------|
| `src/app/globals.css` | Design system lengkap |
| `src/shared/components/Card.js` | Card wrapper |
| `src/shared/components/Button.js` | Button variants |
| `src/shared/components/SegmentedControl.js` | Tab switcher |
| `src/shared/components/layouts/DashboardLayout.js` | Shell layout |
| `src/shared/components/Sidebar.js` | Navigation sidebar |
| `src/app/(dashboard)/dashboard/usage/page.js` | Tab controller pattern |
