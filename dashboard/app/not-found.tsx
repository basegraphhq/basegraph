import Link from "next/link"
import type { Metadata } from "next"

export const metadata: Metadata = {
  title: "Page Not Found",
  description: "The page you're looking for doesn't exist.",
}

export default function NotFound() {
  return (
    <main className="relative min-h-screen bg-background text-foreground flex items-center justify-center px-6">
      <div className="max-w-2xl text-center">
        <h1 className="text-6xl md:text-8xl font-serif font-bold mb-6">404</h1>
        <h2 className="text-2xl md:text-3xl font-serif font-bold mb-4">
          Page Not Found
        </h2>
        <p className="text-lg text-foreground/80 mb-8">
          The page you're looking for doesn't exist or has been moved.
        </p>
        <Link
          href="/"
          className="inline-flex items-center justify-center rounded-md bg-primary px-8 py-3 text-sm font-medium text-primary-foreground shadow transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        >
          Return Home
        </Link>
      </div>
    </main>
  )
}

