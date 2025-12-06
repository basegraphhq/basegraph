'use client'

export function Logo({ className = "" }: { className?: string }) {
  return (
    <svg
      width="28"
      height="28"
      viewBox="0 0 100 100"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-label="Relay logo"
    >
      {/* Central node */}
      <circle cx="50" cy="50" r="4" fill="currentColor" />

      {/* Top arm */}
      <line
        x1="50"
        y1="50"
        x2="50"
        y2="32"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="50" cy="32" r="3.5" fill="currentColor" />

      {/* Upper-right branched arm */}
      <line
        x1="50"
        y1="50"
        x2="60"
        y2="40"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="60" cy="40" r="3.5" fill="currentColor" />
      <line
        x1="60"
        y1="40"
        x2="70"
        y2="38"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="70" cy="38" r="3.5" fill="currentColor" />

      {/* Right arm */}
      <line
        x1="50"
        y1="50"
        x2="69"
        y2="50"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="69" cy="50" r="3.5" fill="currentColor" />

      {/* Bottom-right arm */}
      <line
        x1="50"
        y1="50"
        x2="59"
        y2="63"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="59" cy="63" r="3.5" fill="currentColor" />

      {/* Bottom-left arm */}
      <line
        x1="50"
        y1="50"
        x2="45"
        y2="66"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="45" cy="66" r="3.5" fill="currentColor" />

      {/* Left arm */}
      <line
        x1="50"
        y1="50"
        x2="39"
        y2="50"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
      />
      <circle cx="39" cy="50" r="3.5" fill="currentColor" />
    </svg>
  )
}

