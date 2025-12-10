# Relay Design System

> A refined, editorial design system built for precision and clarity.

## Design Philosophy

**"Refined Paper"** — The aesthetic of premium uncoated stock. High-end architectural journals, museum catalogs, quality letterpress. Warmth is present but nearly imperceptible. Confidence through restraint.

### Core Principles

1. **Restraint over decoration** — Every element earns its place
2. **Hierarchy through contrast** — Not through color variety
3. **Semantic color usage** — Accent colors have meaning, not just aesthetics
4. **Dark mode as first-class** — Both modes are intentionally designed, not inverted
5. **Subtle depth** — Glassmorphism inspired by Apple's liquid glass: felt, not seen

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

### Form Spacing (Vercel-inspired rhythm)

- Label → input: `8px` (use `space-y-2`)
- Input → helper text: `12px` (e.g., `space-y-2` plus a small `mt-1`)
- Between field groups: `24px` (`space-y-6`)
- CTA spacing: at least `24px` above primary actions
- Keep helper text at `text-sm` with `leading-5` for legibility

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

### Overlays & Panels

All overlay components (Sheet, Dialog, AlertDialog) follow a consistent glassmorphism treatment inspired by Apple's liquid glass — subtle depth you *feel* rather than *see*.

**Overlay backdrop:**
```css
bg-black/25 backdrop-blur-[2px]
```

- `bg-black/25` — Light scrim, content remains visible
- `backdrop-blur-[2px]` — Barely perceptible blur, just softens edges

**Panel styling:**

| Component | Rounded Corners | Shadow |
|-----------|-----------------|--------|
| Sheet (right) | `rounded-l-xl` | `shadow-xl` |
| Sheet (left) | `rounded-r-xl` | `shadow-xl` |
| Sheet (top) | `rounded-b-xl` | `shadow-xl` |
| Sheet (bottom) | `rounded-t-xl` | `shadow-xl` |
| Dialog | `rounded-xl` | `shadow-xl` |
| AlertDialog | `rounded-xl` | `shadow-xl` |
| Popover | `rounded-lg` | `shadow-lg` |
| DropdownMenu | `rounded-lg` | `shadow-lg` |

**Why subtle blur?**
Heavy blur (like `backdrop-blur-md`) creates a frosted glass effect that feels dated and draws attention to the overlay itself. Apple's approach uses minimal blur so the *content panel* does the visual heavy lifting, not the backdrop.

```tsx
{/* Sheet for detailed setup flows */}
<Sheet>
  <SheetTrigger asChild>
    <Button>Open Panel</Button>
  </SheetTrigger>
  <SheetContent side="right" className="sm:max-w-[28rem]">
    {/* Content */}
  </SheetContent>
</Sheet>

{/* Dialog for focused actions */}
<Dialog>
  <DialogTrigger asChild>
    <Button>Confirm</Button>
  </DialogTrigger>
  <DialogContent>
    {/* Content */}
  </DialogContent>
</Dialog>
```

### Fixed Positioning

```css
.fixed-top-right          /* Theme toggle position */
.fixed-top-right-offset   /* Login button (offset to not overlap) */
```

Both handle iOS safe areas automatically.

---

## Border Radius & Shadows

### Radius Scale

| Token | Value | Usage |
|-------|-------|-------|
| `rounded-sm` | `calc(var(--radius) - 2px)` | Small elements, close buttons |
| `rounded-md` | `var(--radius)` | Default, inputs |
| `rounded-lg` | `calc(var(--radius) + 2px)` | Cards, dropdowns, popovers |
| `rounded-xl` | `calc(var(--radius) + 4px)` | Dialogs, sheets, elevated panels |

**Convention:**
- Floating elements (popovers, dropdowns) use `rounded-lg`
- Modal overlays (dialogs, sheets) use `rounded-xl`
- Inputs and small controls use `rounded-md`

### Shadow Scale

| Class | Usage |
|-------|-------|
| `shadow-sm` | Buttons at rest |
| `shadow-md` | Buttons on hover, small elevations |
| `shadow-lg` | Popovers, dropdowns |
| `shadow-xl` | Dialogs, sheets, major overlays |

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
├── layout.tsx           # Root layout
├── page.tsx             # Landing page
├── dashboard/
│   ├── layout.tsx       # Dashboard layout
│   ├── page.tsx         # Dashboard home
│   └── onboarding/
│       └── page.tsx     # Onboarding flow
└── api/                 # API routes

components/
├── ui/                  # Primitive components (shadcn/ui based)
│   ├── button.tsx
│   ├── card.tsx
│   ├── dialog.tsx
│   ├── sheet.tsx
│   ├── input.tsx
│   └── ...
├── [feature].tsx        # Feature components (e.g., gitlab-connect-panel.tsx)
└── [shared].tsx         # Shared components (e.g., logo.tsx, top-bar.tsx)

lib/
├── utils.ts             # cn() and other utilities
├── auth.ts              # Auth helpers
└── config.ts            # Environment config

hooks/
├── use-toast.ts         # Toast notifications
└── use-mobile.ts        # Mobile detection
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
- [ ] Do overlays use subtle blur (`backdrop-blur-[2px]`)?
- [ ] Are rounded corners appropriate for the elevation level?

---

## Examples

### Correct Usage

```tsx
// ✅ Good: Uses design system
<p className="text-body-secondary">Supporting text</p>
<Button className="state-connected">Synced</Button>
<div className="fixed-top-right">...</div>

// ✅ Good: Overlay with subtle blur (handled by components)
<Sheet>
  <SheetContent side="right">...</SheetContent>
</Sheet>

// ✅ Good: Appropriate shadow for elevation
<Popover>  {/* Uses shadow-lg */}
<Dialog>   {/* Uses shadow-xl */}
```

### Incorrect Usage

```tsx
// ❌ Bad: Hardcoded values
<p className="text-gray-600">Supporting text</p>
<p style={{ color: 'rgba(0,0,0,0.8)' }}>Text</p>
<Button className="bg-emerald-500/10">Synced</Button>

// ❌ Bad: Heavy blur on overlays
<div className="backdrop-blur-md bg-black/50">  {/* Too heavy */}

// ❌ Bad: Inconsistent rounded corners
<div className="rounded-3xl">  {/* Not in our scale */}
```

---

## Component Conventions

### When to use Sheet vs Dialog

| Use Case | Component | Why |
|----------|-----------|-----|
| Setup flows with multiple steps | Sheet | More space, user can reference while doing external tasks |
| Quick confirmations | Dialog | Focused, centered attention |
| Forms with many fields | Sheet | Scrollable, doesn't feel cramped |
| Destructive action confirmation | AlertDialog | Requires explicit decision |
| Settings or detail views | Sheet | Companion panel feel |

### Sheet Width Guidelines

| Content Type | Width | Class |
|--------------|-------|-------|
| Simple forms | 384px | `sm:max-w-sm` (default) |
| Setup guides with instructions | 448px | `sm:max-w-[28rem]` |
| Complex forms or previews | 512px | `sm:max-w-md` |

---

*Last updated: December 2024*

