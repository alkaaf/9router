# Atomic Task: fe-6 — Apply Styling & Responsive Layout

**Domain**: Frontend
**Priority**: Medium
**Estimated effort**: 20 min

---

## Input

- `PerKeyUsagePage` component (fe-2 through fe-5 assembled)
- Tailwind CSS classes from existing design system
- `Card`, `SegmentedControl`, `UsageTable` shared components

## Output

- Page fully styled matching existing dashboard design
- Responsive on mobile and desktop
- Dark/light mode compatible

## Process

### Step 1: Verify root container

Root div should have:
```jsx
<div className="flex min-w-0 flex-col gap-6 px-1 sm:px-0">
```
- `min-w-0` enables text truncation in flex children
- `px-1 sm:px-0` — small horizontal padding on mobile, none on desktop
- `gap-6` — consistent spacing between sections

### Step 2: Verify period selector responsiveness

```jsx
<SegmentedControl ... className="w-full sm:w-auto" />
```
- Full width on mobile, auto width on desktop

### Step 3: Verify chart card responsiveness

```jsx
<Card className="flex min-w-0 flex-col gap-3 p-3 sm:p-4">
```
- Smaller padding on mobile (`p-3`), larger on desktop (`sm:p-4`)
- `ResponsiveContainer width="100%"` for chart width adaptation

### Step 4: Verify table responsiveness

- `UsageTable` component handles horizontal scroll on small screens internally
- No additional changes needed

### Step 5: Verify header styling

```jsx
<h1 className="text-xl font-bold">Usage per User</h1>
<p className="text-sm text-text-muted">
  {data.keyName || "Unnamed Key"} &middot; <code className="text-xs bg-surface-2 px-1.5 py-0.5 rounded">{data.keyMasked}</code>
</p>
```
- `text-xl font-bold` for page title
- `text-sm text-text-muted` for subtitle
- `<code>` badge with `bg-surface-2` background

### Step 6: Verify CSS variable usage in chart

All chart colors use CSS variables or standard hex:
- `var(--color-bg)` — background
- `var(--color-border)` — border
- `#6366f1` — indigo (tokens)
- `#f59e0b` — amber (cost)

These work in both dark and light modes.

## Dependencies

- fe-5: UsageTable integrated (DONE)

## Success Criteria

- Page looks consistent with existing Usage dashboard
- Mobile: stacked layout, full-width controls
- Desktop: horizontal controls, comfortable spacing
- No overflow or layout breakage at 320px width
- Dark/light mode both render correctly
- All text colors use semantic tokens (`text-text`, `text-text-muted`, etc.)
