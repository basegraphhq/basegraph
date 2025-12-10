"use client"

import { useSession } from "@/hooks/use-session"
import { Typewriter } from "@/components/typewriter"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Check, Gitlab } from "lucide-react"
import { cn } from "@/lib/utils"
import { GitLabConnectPanel } from "@/components/gitlab-connect-panel"

const messages = [
  "Connect your tools to enable Relay's code analysis and context gathering.",
  "Then generate production-ready specs from your repositories in seconds.",
]

export default function DashboardPage() {
  const { data: session, isPending } = useSession()
  const router = useRouter()
  
  const [gitlabConnected, setGitlabConnected] = useState(false)
  const [showCard, setShowCard] = useState(false)

  useEffect(() => {
    if (!isPending && !session) {
      router.push("/")
    }
  }, [isPending, session, router])

  const handleGitlabConnect = (data: { instanceUrl: string; token: string }) => {
    console.log('GitLab connected:', data.instanceUrl)
    setGitlabConnected(true)
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
          {/* GitLab Integration */}
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
            ) : (
              <GitLabConnectPanel onConnect={handleGitlabConnect}>
                <Button 
                  variant="outline"
                  size="sm"
                  className="min-w-[90px] h-8 text-xs font-medium hover:bg-primary hover:text-primary-foreground"
                >
                  Connect
                </Button>
              </GitLabConnectPanel>
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
