"use client"

import { usePathname } from "next/navigation"
import Link from "next/link"
import { ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"

interface BreadcrumbItem {
  label: string
  href: string
  isCurrentPage?: boolean
}

// Map route segments to display labels
const segmentLabels: Record<string, string> = {
  dashboard: "Home",
  integrations: "Integrations",
  settings: "Settings",
}

function generateBreadcrumbs(pathname: string): BreadcrumbItem[] {
  const segments = pathname.split("/").filter(Boolean)
  const breadcrumbs: BreadcrumbItem[] = []

  let currentPath = ""

  segments.forEach((segment, index) => {
    currentPath += `/${segment}`
    const isLast = index === segments.length - 1

    // Get the display label (use mapped label or capitalize the segment)
    const label = segmentLabels[segment] || segment.charAt(0).toUpperCase() + segment.slice(1)

    breadcrumbs.push({
      label,
      href: currentPath,
      isCurrentPage: isLast,
    })
  })

  return breadcrumbs
}

export function TopBar() {
  const pathname = usePathname()
  const breadcrumbs = generateBreadcrumbs(pathname)

  return (
    <div 
      className="flex items-center px-6 border-b border-border/40"
      style={{ height: 'var(--header-height-sm)' }}
    >
      <nav aria-label="Breadcrumb">
        <ol className="flex items-center gap-1">
          {breadcrumbs.map((crumb, index) => (
            <li key={crumb.href} className="flex items-center">
              {index > 0 && (
                <ChevronRight className="size-3.5 mx-1 text-muted-foreground/60" />
              )}
              {crumb.isCurrentPage ? (
                <span className="text-caption font-serif font-medium text-foreground">
                  {crumb.label}
                </span>
              ) : (
                <Link
                  href={crumb.href}
                  className={cn(
                    "text-caption text-muted-foreground hover:text-foreground",
                    "transition-colors"
                  )}
                  style={{ transitionDuration: 'var(--duration-fast)' }}
                >
                  {crumb.label}
                </Link>
              )}
            </li>
          ))}
        </ol>
      </nav>
    </div>
  )
}
