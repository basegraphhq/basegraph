# Basegraph Design System

> A refined, editorial design system built for precision and clarity.

## Design Philosophy

**"Refined Paper"** — The aesthetic of premium uncoated stock. High-end architectural journals, museum catalogs, quality letterpress. Warmth is present but nearly imperceptible. Confidence through restraint.

### Core Principles

1. **Restraint over decoration** — Every element earns its place
2. **Hierarchy through contrast** — Not through color variety
3. **Semantic color usage** — Accent colors have meaning, not just aesthetics
4. **Dark mode as first-class** — Both modes are intentionally designed, not inverted

---

## Color System

### Understanding OKLCH

We use OKLCH (Oklab Lightness Chroma Hue) for all colors:

```
oklch(L C H)
│     │ │ └─ Hue: 0-360° color wheel (0=pink, 60=yellow, 120=green, 240=blue)
│     │ └─── Chroma: 0=grey, higher=more saturated
│     └───── Lightness: 0=black, 1=white
```

**Key insight**: Elite off-white means chroma under `0.005`. At this level, color reads as "not quite white" without looking tinted.

### Light Mode Palette

| Token | Value | Usage |
|-------|-------|-------|
| `--background` | `oklch(0.978 0.003 70)` | Page background — warm paper |
| `--foreground` | `oklch(0.15 0.008 50)` | Primary text — deep ink |
| `--card` | `oklch(0.99 0.002 70)` | Elevated surfaces — cleaner |
| `--muted` | `oklch(0.945 0.004 70)` | Subtle backgrounds |
| `--muted-foreground` | `oklch(0.42 0.008 50)` | Secondary text |
| `--accent` | `oklch(0.52 0.105 35)` | Sienna — brand accent |
| `--border` | `oklch(0.915 0.004 70)` | Subtle borders |

### Dark Mode Palette

| Token | Value | Usage |
|-------|-------|-------|
| `--background` | `oklch(0.14 0.004 50)` | Deep charcoal with warmth |
| `--foreground` | `oklch(0.94 0.003 70)` | Off-white text |
| `--card` | `oklch(0.17 0.004 50)` | Slightly lifted |
| `--muted` | `oklch(0.22 0.004 50)` | Subtle backgrounds |
| `--muted-foreground` | `oklch(0.62 0.004 70)` | Secondary text |
| `--accent` | `oklch(0.60 0.12 38)` | Sienna — brighter for dark |
| `--border` | `oklch(0.24 0.004 50)` | Subtle borders |

### Semantic Colors

| Token | Purpose | Light | Dark |
|-------|---------|-------|------|
| `--success` | Connected, complete | `oklch(0.52 0.10 150)` | `oklch(0.58 0.11 150)` |
| `--warning` | Caution states | `oklch(0.60 0.13 75)` | `oklch(0.68 0.13 75)` |
| `--destructive` | Errors, danger | `oklch(0.50 0.14 25)` | `oklch(0.55 0.14 25)` |
| `--info` | Informational | `oklch(0.52 0.09 245)` | `oklch(0.58 0.10 245)` |

### Color Usage Rules

```
┌─────────────────────────────────────────────────────────────┐
│  BLACK (--primary)         │  Main CTAs, primary buttons   │
│  SIENNA (--accent)         │  Highlights, links, status    │
│  MUTED GREY                │  System chrome, menus         │
│  SEMANTIC COLORS           │  States only (success, error) │
└─────────────────────────────────────────────────────────────┘
```

**Sienna is reserved for:**
- Primary links you want clicked
- Active/selected states  
- Status indicators (the pulse dot)
- Hover states on outline buttons

**Sienna is NOT for:**
- Utility actions (logout, theme toggle)
- Menu item hovers
- System chrome

---

## Typography

### Font Stack

```css
--font-serif: "Libertinus Serif", Georgia, serif;     /* Headings, body */
--font-mono: "Geist Mono", "Fira Code", monospace;    /* Code, technical */
--font-sans: system-ui, -apple-system, sans-serif;    /* UI labels */
```

### Type Scale

| Class | Size | Usage |
|-------|------|-------|
| `.text-display` | `clamp(3rem, 8vw, 4.5rem)` | Hero headlines |
| `.h1` / `h1` | `2.25rem` (36px) | Page titles |
| `.h2` / `h2` | `1.875rem` (30px) | Section headings |
| `.h3` / `h3` | `1.5rem` (24px) | Subsection headings |
| `.h4` / `h4` | `1.25rem` (20px) | Card titles |
| `.text-lead` | `1.25rem` | Intro paragraphs |
| `.text-base` | `1rem` (16px) | Body text |
| `.text-sm` | `0.875rem` (14px) | Secondary text |
| `.text-xs` | `0.75rem` (12px) | Captions |

### Text Utilities

```css
.text-body-secondary  /* color: foreground, opacity: 0.8 */
.text-body-tertiary   /* color: foreground, opacity: 0.7 */
.text-link            /* foreground → accent on hover */
.text-overline        /* Uppercase, spaced, mono */
.text-caption         /* Sans-serif, small */
.text-mono            /* Monospace, tight tracking */
```

---

## Spacing

Based on a 4px grid:

| Token | Value | Pixels |
|-------|-------|--------|
| `--space-1` | `0.25rem` | 4px |
| `--space-2` | `0.5rem` | 8px |
| `--space-3` | `0.75rem` | 12px |
| `--space-4` | `1rem` | 16px |
| `--space-6` | `1.5rem` | 24px |
| `--space-8` | `2rem` | 32px |
| `--space-12` | `3rem` | 48px |
| `--space-16` | `4rem` | 64px |
| `--space-20` | `5rem` | 80px |

### Layout Tokens

```css
--page-padding: var(--space-6);           /* Horizontal page margins */
--section-padding: var(--space-20);       /* Vertical section spacing */
--component-gap: var(--space-4);          /* Default gap between elements */
--header-height: 3.5rem;                  /* Top bar height */
```

---

## Layout Utilities

### Containers

```css
.page-container      /* max-width: 56rem, centered, padded */
.page-container-sm   /* max-width: 40rem */
.page-container-md   /* max-width: 48rem */
.page-container-xl   /* max-width: 72rem */
```

### Flex Patterns

```css
.stack               /* Vertical flex, gap: --component-gap */
.stack-lg            /* Vertical flex, gap: --component-gap-lg */
.cluster             /* Horizontal flex wrap, centered */
```

### Spacing

```css
.section-padding     /* Vertical padding for sections */
.content-spacing     /* Padding for content areas */
```

---

## Component Patterns

### Buttons

| Variant | Usage | Appearance |
|---------|-------|------------|
| `default` | Primary CTA | Black, shadow, lifts on hover |
| `secondary` | Important secondary | Sienna background |
| `outline` | Tertiary actions | Border only, accent on hover |
| `ghost` | Minimal UI | Invisible until hover |
| `link` | Inline links | Underline, accent color |
| `destructive` | Danger actions | Red tones |

```tsx
<Button>Join Waitlist</Button>                    {/* Primary */}
<Button variant="secondary">View Demo</Button>    {/* Secondary */}
<Button variant="outline">Settings</Button>       {/* Tertiary */}
<Button variant="ghost">Cancel</Button>           {/* Minimal */}
```

### State Classes

```css
.state-connected     /* Green success state — "Synced" buttons */
.state-pending       /* Muted loading state */
```

```tsx
<Button className={cn("min-w-[90px]", isConnected && "state-connected")}>
  {isConnected ? "Synced" : "Connect"}
</Button>
```

### Interactive Row

For list items, integration cards, settings rows:

```css
.interactive-row     /* flex, gap, padding, hover:bg-muted */
```

```tsx
<div className="interactive-row">
  <div className="icon-container icon-container-md">{icon}</div>
  <div className="flex-1">
    <h4>Title</h4>
    <p className="text-muted-foreground">Description</p>
  </div>
  <Button>Action</Button>
</div>
```

### Cards

```tsx
{/* Standard card */}
<Card>Content</Card>

{/* Subtle/transparent card */}
<Card className="card-subtle">Content</Card>
```

### Fixed Positioning

```css
.fixed-top-right          /* Theme toggle position */
.fixed-top-right-offset   /* Login button (offset to not overlap) */
```

Both handle iOS safe areas automatically.

---

## Animation

### Duration Tokens

```css
--duration-fast: 150ms;
--duration-normal: 250ms;
--duration-slow: 400ms;
```

### Easing

```css
--ease-default: cubic-bezier(0.4, 0, 0.2, 1);
--ease-in: cubic-bezier(0.4, 0, 1, 1);
--ease-out: cubic-bezier(0, 0, 0.2, 1);
```

### Animation Classes

```css
.animate-fade-in     /* Fade in */
.animate-slide-up    /* Fade + slide from bottom */
.animate-blink       /* Cursor blink */
```

### Button Interactions

Buttons have built-in micro-interactions:
- `hover:shadow-md` — Lifts on hover
- `active:scale-[0.98]` — Subtle press feedback
- `transition-all duration-200` — Smooth transitions

---

## Accessibility

### Focus States

All interactive elements have visible focus indicators:
```css
focus-visible:ring-ring/50 focus-visible:ring-[3px]
```

### Color Contrast

- Text on background: Minimum 4.5:1 ratio
- Large text (h1-h3): Minimum 3:1 ratio
- Interactive elements: Clear hover/focus states

### Safe Areas

iOS safe area insets are handled via:
```css
padding-top: env(safe-area-inset-top);
/* etc. */
```

---

## File Structure

```
app/
├── globals.css          # Design tokens, base styles, utilities
├── page.tsx             # Landing page
└── dashboard/
    └── page.tsx         # Dashboard

components/
├── ui/                  # Primitive components (Button, Card, Input...)
│   ├── button.tsx
│   ├── card.tsx
│   └── ...
└── [feature].tsx        # Feature components

styles/
└── globals.css          # Legacy (imports handled in app/globals.css)
```

---

## Quick Reference

### Adding New Colors

1. Define in `:root` and `.dark` in `globals.css`
2. Map to Tailwind in `@theme inline` block
3. Use via `bg-{color}`, `text-{color}`, etc.

### Creating New Utilities

Add to `@layer components` in `globals.css`:

```css
@layer components {
  .my-utility {
    /* styles using CSS variables */
    color: var(--foreground);
    padding: var(--space-4);
  }
}
```

### Design Decisions Checklist

Before adding UI:
- [ ] Does it use design tokens (not hardcoded values)?
- [ ] Does it work in both light and dark mode?
- [ ] Is the color usage semantic (not decorative)?
- [ ] Does it follow the typography scale?
- [ ] Are interactive states clear?

---

## Examples

### Correct Usage

```tsx
// ✅ Good: Uses design system
<p className="text-body-secondary">Supporting text</p>
<Button className="state-connected">Synced</Button>
<div className="fixed-top-right">...</div>
```

### Incorrect Usage

```tsx
// ❌ Bad: Hardcoded values
<p className="text-gray-600">Supporting text</p>
<p style={{ color: 'rgba(0,0,0,0.8)' }}>Text</p>
<Button className="bg-emerald-500/10">Synced</Button>
<div style={{ top: 'calc(1.5rem + env(...))' }}>...</div>
```

---

*Last updated: December 2024*

