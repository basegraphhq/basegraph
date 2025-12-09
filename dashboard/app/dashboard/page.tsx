"use client"

import { useSession } from "@/lib/auth-client"
import { Typewriter } from "@/components/typewriter"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useEffect, useRef, useState } from "react"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { RefreshCw, Check, Gitlab, ChevronDown, Server } from "lucide-react"
import { cn } from "@/lib/utils"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Input } from "@/components/ui/input"
const messages = [
  "Connect your tools to enable Relay's code analysis and context gathering.",
  "Then generate production-ready specs from your repositories in seconds.",
]

// Wrapper component to handle Suspense boundary for useSearchParams
export default function DashboardPage() {
  return (
    <Suspense fallback={null}>
      <DashboardContent />
    </Suspense>
  )
}

function DashboardContent() {
  const { data: session, isPending } = useSession()
  const router = useRouter()
  const searchParams = useSearchParams()
  const hasSynced = useRef(false)
  const hasCheckedGitlab = useRef(false)
  
  // Integration states
  const [gitlabLoading, setGitlabLoading] = useState(false)
  const [gitlabConnected, setGitlabConnected] = useState(false)
  
  // GitLab instance URL state
  const [gitlabUrl, setGitlabUrl] = useState("https://gitlab.com")
  const [gitlabPopoverOpen, setGitlabPopoverOpen] = useState(false)
  
  // Animation state
  const [showCard, setShowCard] = useState(false)

  // Redirect to home if not authenticated
  useEffect(() => {
    if (!isPending && !session) {
      router.push("/")
    }
  }, [isPending, session, router])

  // Sync user to Relay (runs once per session)
  useEffect(() => {
    if (!isPending && session && !hasSynced.current) {
      hasSynced.current = true
      
      fetch("/api/user/sync", { method: "POST" })
        .then((res) => res.json())
        .then((data) => {
          if (data.error) {
            console.error("Failed to sync user:", data)
          } else if (data.has_organization === false) {
            router.push("/dashboard/onboarding")
          }
        })
        .catch((err) => {
          console.error("Error syncing user:", err)
        })
    }
  }, [isPending, session, router])

  // Check if returning from GitLab OAuth and fetch projects
  useEffect(() => {
    const gitlabConnectedParam = searchParams.get('gitlab_connected')
    const gitlabError = searchParams.get('gitlab_error')
    
    if (gitlabError) {
      console.error('GitLab OAuth error:', gitlabError)
      setGitlabLoading(false)
      // Clear the error from URL
      router.replace('/dashboard')
      return
    }
    
    if (gitlabConnectedParam === 'true' && !hasCheckedGitlab.current) {
      hasCheckedGitlab.current = true
      // Clear the param from URL
      router.replace('/dashboard')
      // Fetch projects
      fetchGitlabProjects()
    }
  }, [searchParams, router])

  // Function to fetch GitLab projects
  const fetchGitlabProjects = async () => {
    setGitlabLoading(true)
    try {
      const response = await fetch('/api/gitlab/projects')
      const data = await response.json()
      
      if (response.ok && data.success) {
        console.log('=== GitLab Projects ===')
        console.log(`Found ${data.count} projects:`)
        data.projects.forEach((project: {
          name: string
          fullPath: string
          description: string | null
          visibility: string
          lastActivityAt: string
        }) => {
          console.log(`- ${project.name} (${project.fullPath})`)
          console.log(`  Description: ${project.description || 'No description'}`)
          console.log(`  Visibility: ${project.visibility}`)
          console.log(`  Last activity: ${project.lastActivityAt}`)
        })
        console.log('======================')
        setGitlabConnected(true)
      } else {
        console.error('Failed to fetch GitLab projects:', data.error)
      }
    } catch (error) {
      console.error('Error fetching GitLab projects:', error)
    } finally {
      setGitlabLoading(false)
    }
  }

  const handleGitlabSync = async (issuerUrl: string = "https://gitlab.com") => {
    setGitlabLoading(true)
    setGitlabPopoverOpen(false)
    
    // First, check if we already have a valid GitLab token by trying to fetch projects
    try {
      const response = await fetch('/api/gitlab/projects')
      const data = await response.json()
      
      if (response.ok && data.success) {
        // Already connected! Just log the projects
        console.log('=== GitLab Projects ===')
        console.log(`Found ${data.count} projects:`)
        data.projects.forEach((project: {
          name: string
          fullPath: string
          description: string | null
          visibility: string
          lastActivityAt: string
        }) => {
          console.log(`- ${project.name} (${project.fullPath})`)
          console.log(`  Description: ${project.description || 'No description'}`)
          console.log(`  Visibility: ${project.visibility}`)
          console.log(`  Last activity: ${project.lastActivityAt}`)
        })
        console.log('======================')
        setGitlabConnected(true)
        setGitlabLoading(false)
        return
      }
    } catch {
      // Not connected or error, proceed with OAuth
    }
    
    // Redirect to GitLab OAuth with the issuer URL
    const params = new URLSearchParams()
    if (issuerUrl !== "https://gitlab.com") {
      params.set('issuer', issuerUrl)
    }
    const queryString = params.toString()
    window.location.href = `/api/gitlab/authorize${queryString ? `?${queryString}` : ''}`
  }

  // Loading state - show nothing while session loads (fast enough to not need skeleton)
  if (isPending) {
    return null
  }

  // Not authenticated (will redirect)
  if (!session) {
    return null
  }

  return (
    <div className="content-spacing max-w-2xl mx-auto">
      {/* Greeting Section */}
      <div className="mb-10 pt-6">
        <Typewriter 
          messages={["Hey " + session.user?.name?.split(" ")[0] + "!"]} 
          className="h3"
          lineClassName="text-foreground"
        />
        <Typewriter 
          messages={messages} 
          className="mt-2" 
          lineClassName="text-lg text-muted-foreground"
          onComplete={() => setShowCard(true)}
        />
      </div>

      {/* Integration Card */}
      <div className={cn(
        "transition-all duration-1000 ease-out",
        showCard ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4"
      )}>
        <Card className="bg-card/50 shadow-none border-border/60 p-2 gap-1">
          {/* GitLab Integration with Instance URL Selector */}
          <div className="interactive-row">
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-background text-foreground ring-1 ring-inset ring-border/50">
              <Gitlab className="size-5" />
            </div>
            <div className="flex-1 min-w-0 grid gap-0.5">
              <h4 className="text-sm font-medium leading-none text-foreground">Sync with GitLab</h4>
              <p className="text-sm text-muted-foreground leading-normal">
                Connect your GitLab repositories to enable code analysis and codebase mapping
              </p>
            </div>
            
            {gitlabConnected ? (
              <Button 
                disabled
                variant="outline"
                size="sm"
                className="min-w-[90px] h-8 text-xs font-medium state-connected"
              >
                <Check className="size-3 mr-1.5" />
                Synced
              </Button>
            ) : gitlabLoading ? (
              <Button 
                disabled
                variant="outline"
                size="sm"
                className="min-w-[90px] h-8 text-xs font-medium"
              >
                <RefreshCw className="size-3 animate-spin mr-1.5" />
                Syncing
              </Button>
            ) : (
              <Popover open={gitlabPopoverOpen} onOpenChange={setGitlabPopoverOpen}>
                <PopoverTrigger asChild>
                  <Button 
                    variant="outline"
                    size="sm"
                    className="min-w-[90px] h-8 text-xs font-medium hover:bg-primary hover:text-primary-foreground group"
                  >
                    Connect
                    <ChevronDown className="size-3 ml-1 opacity-50 group-hover:opacity-100 transition-opacity" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent 
                  align="end" 
                  className="w-80 p-4"
                >
                  <div className="grid gap-4">
                    <div className="space-y-2">
                      <div className="flex items-center gap-2">
                        <Server className="size-4 text-muted-foreground" />
                        <h4 className="font-medium text-sm">GitLab Instance</h4>
                      </div>
                      <p className="text-xs text-muted-foreground">
                        Use gitlab.com or enter your self-hosted instance URL
                      </p>
                    </div>
                    
                    <div className="grid gap-3">
                      <Input
                        type="url"
                        placeholder="https://gitlab.com"
                        value={gitlabUrl}
                        onChange={(e) => setGitlabUrl(e.target.value)}
                        className="h-9 text-sm font-mono"
                      />
                      
                      {/* Quick preset buttons */}
                      <div className="flex gap-2">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className={cn(
                            "h-7 text-xs flex-1",
                            gitlabUrl === "https://gitlab.com" && "bg-muted"
                          )}
                          onClick={() => setGitlabUrl("https://gitlab.com")}
                        >
                          GitLab.com
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className={cn(
                            "h-7 text-xs flex-1",
                            gitlabUrl !== "https://gitlab.com" && gitlabUrl !== "" && "bg-muted"
                          )}
                          onClick={() => {
                            if (gitlabUrl === "https://gitlab.com") {
                              setGitlabUrl("https://")
                            }
                          }}
                        >
                          Self-hosted
                        </Button>
                      </div>
                    </div>
                    
                    <Button 
                      onClick={() => handleGitlabSync(gitlabUrl)}
                      disabled={!gitlabUrl || !gitlabUrl.startsWith('https://')}
                      className="w-full"
                      size="sm"
                    >
                      <Gitlab className="size-4 mr-2" />
                      Connect to GitLab
                    </Button>
                  </div>
                </PopoverContent>
              </Popover>
            )}
          </div>
        </Card>

        {/* Coming soon integrations */}
        <div className="mt-6 pt-6 border-t border-border/40">
          <p className="text-caption text-muted-foreground mb-4">Coming soon</p>
          <div className="flex flex-wrap gap-3">
            {["Github", "Linear", "Jira"].map((integration) => (
              <span 
                key={integration}
                className="px-3 py-1.5 text-xs text-muted-foreground bg-muted/50 rounded-md"
              >
                {integration}
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
