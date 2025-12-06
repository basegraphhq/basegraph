"use client"

import { useSession, signOut } from "@/lib/auth-client"
import { useTheme } from "next-themes"
import { useRouter } from "next/navigation"
import * as React from "react"
import Link from "next/link"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { LogOut, Moon, Sun, ChevronDown } from "lucide-react"
import { cn } from "@/lib/utils"

export function TopNavBar() {
  const { data: session } = useSession()
  const router = useRouter()
  const { theme, setTheme } = useTheme()
  const [mounted, setMounted] = React.useState(false)

  React.useEffect(() => {
    setMounted(true)
  }, [])

  const handleSignOut = async () => {
    await signOut()
    router.push("/")
  }

  return (
    <header 
      className="flex items-center justify-between px-6 border-b border-border/40 bg-background"
      style={{ height: 'var(--header-height)' }}
    >
      {/* Logo / Brand */}
      <Link 
        href="/dashboard" 
        className="flex items-center gap-2 text-foreground hover:opacity-80 transition-opacity"
        style={{ transitionDuration: 'var(--duration-fast)' }}
      >
        <span className="h4">Basegraph</span>
      </Link>

      {/* Right side: User dropdown */}
      <div className="flex items-center">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button 
              className={cn(
                "flex items-center gap-2 rounded-md px-2 py-1.5",
                "hover:bg-muted/50 transition-colors",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
              )}
              style={{ transitionDuration: 'var(--duration-fast)' }}
            >
              <Avatar className="h-7 w-7">
                <AvatarImage src={session?.user?.image || ""} alt={session?.user?.name || ""} />
                <AvatarFallback className="text-xs bg-muted">
                  {session?.user?.name?.slice(0, 2).toUpperCase() || "U"}
                </AvatarFallback>
              </Avatar>
              <span className="text-sm font-medium text-foreground hidden sm:block">
                {session?.user?.name?.split(" ")[0] || "User"}
              </span>
              <ChevronDown className="size-3 text-muted-foreground" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent 
            align="end" 
            sideOffset={8}
            className="w-48"
          >
            {/* User info */}
            <div className="px-2 py-2 border-b border-border/40">
              <p className="text-sm font-medium text-foreground truncate">
                {session?.user?.name || "User"}
              </p>
              <p className="text-xs text-muted-foreground truncate">
                {session?.user?.email || "email@example.com"}
              </p>
            </div>
            
            <DropdownMenuItem
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              className="cursor-pointer mt-1"
            >
              {mounted && theme === "dark" ? (
                <Sun className="size-4 mr-2" />
              ) : (
                <Moon className="size-4 mr-2" />
              )}
              {mounted && theme === "dark" ? "Light mode" : "Dark mode"}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onClick={handleSignOut}
              className="cursor-pointer"
            >
              <LogOut className="size-4 mr-2" />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  )
}

