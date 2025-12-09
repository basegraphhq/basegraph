'use client'

import { useState } from 'react'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  ChevronDown,
  ExternalLink,
  Eye,
  EyeOff,
  Check,
  Loader2,
  AlertCircle,
  User,
  Key,
} from 'lucide-react'
import { cn } from '@/lib/utils'

interface GitLabConnectPanelProps {
  children: React.ReactNode
  onConnect?: (data: { instanceUrl: string; token: string }) => void
}

export function GitLabConnectPanel({ children, onConnect }: GitLabConnectPanelProps) {
  const [open, setOpen] = useState(false)
  const [step1Open, setStep1Open] = useState(true)
  const [step2Open, setStep2Open] = useState(false)
  
  const [instanceUrl, setInstanceUrl] = useState('https://gitlab.com')
  const [token, setToken] = useState('')
  const [showToken, setShowToken] = useState(false)
  
  const [testStatus, setTestStatus] = useState<'idle' | 'loading' | 'success' | 'error'>('idle')
  const [testResult, setTestResult] = useState<{ username?: string; projectCount?: number } | null>(null)
  const [connectLoading, setConnectLoading] = useState(false)

  const isValidUrl = instanceUrl.startsWith('https://') && instanceUrl.length > 10
  const isValidToken = token.length > 10

  const handleTestConnection = async () => {
    if (!isValidUrl || !isValidToken) return
    
    setTestStatus('loading')
    setTestResult(null)

    try {
      const res = await fetch('/api/integrations/gitlab/test-connection', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          instance_url: instanceUrl,
          token,
        }),
      })

      if (!res.ok) {
        setTestStatus('error')
        return
      }

      const data: { username?: string; project_count?: number } = await res.json()
      setTestStatus('success')
      setTestResult({
        username: data.username ?? 'unknown',
        projectCount: data.project_count ?? 0,
      })
    } catch (error) {
      console.error('Failed to test GitLab connection', error)
      setTestStatus('error')
    }
  }

  const handleConnect = async () => {
    if (!isValidUrl || !isValidToken || testStatus !== 'success') return
    
    setConnectLoading(true)
    
    // Mock API call
    await new Promise(resolve => setTimeout(resolve, 1000))
    
    onConnect?.({ instanceUrl, token })
    setConnectLoading(false)
    setOpen(false)
    
    // Reset state
    setToken('')
    setTestStatus('idle')
    setTestResult(null)
  }

  const resetState = () => {
    setStep1Open(true)
    setStep2Open(false)
    setInstanceUrl('https://gitlab.com')
    setToken('')
    setShowToken(false)
    setTestStatus('idle')
    setTestResult(null)
  }

  return (
    <Sheet open={open} onOpenChange={(isOpen) => {
      setOpen(isOpen)
      if (!isOpen) resetState()
    }}>
      <SheetTrigger asChild>
        {children}
      </SheetTrigger>
      <SheetContent 
        side="right" 
        className="w-full sm:max-w-[28rem] flex flex-col p-0 overflow-hidden"
      >
        <SheetHeader className="px-6 pt-6 pb-4 shrink-0">
          <SheetTitle>Connect GitLab</SheetTitle>
          <SheetDescription>
            Set up a service account and personal access token to connect your GitLab instance.
          </SheetDescription>
        </SheetHeader>
        
        <Separator className="shrink-0" />
        
        <div className="flex-1 overflow-y-auto">
          {/* Instructions Section */}
          <div className="px-6 py-6 space-y-4">
            {/* Step 1: Create Service Account */}
            <Collapsible open={step1Open} onOpenChange={setStep1Open}>
              <CollapsibleTrigger className="flex items-center gap-3 w-full text-left group">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                  <User className="size-4" />
                </div>
                <div className="flex-1 min-w-0">
                  <h4 className="text-sm font-medium text-foreground">Create a Service Account</h4>
                  <p className="text-xs text-muted-foreground">Dedicated bot user for Relay</p>
                </div>
                <ChevronDown className={cn(
                  "size-4 text-muted-foreground transition-transform duration-200",
                  step1Open && "rotate-180"
                )} />
              </CollapsibleTrigger>
              
              <CollapsibleContent className="pt-4 pl-11 space-y-4">
                <div className="text-sm text-muted-foreground space-y-3">
                  <p>
                    We recommend creating a dedicated service account (bot user) rather than using your personal account. This ensures:
                  </p>
                  <ul className="list-disc list-inside space-y-1.5 text-sm">
                    <li>Access isn't tied to any individual</li>
                    <li>Clearer audit trails in GitLab</li>
                    <li>Easier permission management</li>
                  </ul>
                  <div className="pt-2 space-y-2">
                    <p className="font-medium text-foreground">To create a service account:</p>
                    <ol className="list-decimal list-inside space-y-1.5">
                      <li>Create a new GitLab user called <code className="text-mono text-xs bg-muted px-1.5 py-0.5 rounded">relay</code></li>
                      <li>Add it to your group/projects with <strong>Developer</strong> role</li>
                      <li>Use this account to generate the access token</li>
                    </ol>
                  </div>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  onClick={() => window.open(`${instanceUrl}/admin/users/new`, '_blank')}
                >
                  Open GitLab Admin
                  <ExternalLink className="size-3" />
                </Button>
              </CollapsibleContent>
            </Collapsible>

            {/* Step 2: Create Personal Access Token */}
            <Collapsible open={step2Open} onOpenChange={setStep2Open}>
              <CollapsibleTrigger className="flex items-center gap-3 w-full text-left group">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                  <Key className="size-4" />
                </div>
                <div className="flex-1 min-w-0">
                  <h4 className="text-sm font-medium text-foreground">Create Personal Access Token</h4>
                  <p className="text-xs text-muted-foreground">Required scope: api</p>
                </div>
                <ChevronDown className={cn(
                  "size-4 text-muted-foreground transition-transform duration-200",
                  step2Open && "rotate-180"
                )} />
              </CollapsibleTrigger>
              
              <CollapsibleContent className="pt-4 pl-11 space-y-4">
                <div className="text-sm text-muted-foreground space-y-3">
                  <p>
                    Generate a Personal Access Token (PAT) from the service account:
                  </p>
                  <ol className="list-decimal list-inside space-y-1.5">
                    <li>Go to <strong>User Settings → Access Tokens</strong></li>
                    <li>Name it something recognizable (e.g., <code className="text-mono text-xs bg-muted px-1.5 py-0.5 rounded">relay-integration</code>)</li>
                    <li>Select the <code className="text-mono text-xs bg-muted px-1.5 py-0.5 rounded">api</code> scope</li>
                    <li>Set an expiration date (or leave blank for no expiry)</li>
                    <li>Click <strong>Create personal access token</strong></li>
                  </ol>
                  <p className="text-xs text-warning bg-warning/10 px-3 py-2 rounded-md">
                    Copy the token immediately — GitLab only shows it once.
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-2"
                  onClick={() => window.open(`${instanceUrl}/-/user_settings/personal_access_tokens`, '_blank')}
                >
                  Open Access Tokens
                  <ExternalLink className="size-3" />
                </Button>
              </CollapsibleContent>
            </Collapsible>
          </div>
          
          <Separator />
          
          {/* Form Section */}
          <div className="px-6 py-6 space-y-4 bg-muted/30">
            <div className="space-y-2">
              <Label htmlFor="gitlab-url">GitLab Instance URL</Label>
              <Input
                id="gitlab-url"
                type="url"
                placeholder="https://gitlab.com"
                value={instanceUrl}
                onChange={(e) => {
                  setInstanceUrl(e.target.value)
                  setTestStatus('idle')
                  setTestResult(null)
                }}
                className="font-mono text-sm"
              />
            </div>
            
            <div className="space-y-2">
              <Label htmlFor="gitlab-token">Personal Access Token</Label>
              <div className="relative">
                <Input
                  id="gitlab-token"
                  type={showToken ? 'text' : 'password'}
                  placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
                  value={token}
                  onChange={(e) => {
                    setToken(e.target.value)
                    setTestStatus('idle')
                    setTestResult(null)
                  }}
                  className="font-mono text-sm pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowToken(!showToken)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                >
                  {showToken ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </button>
              </div>
            </div>

            {/* Test Result */}
            {testStatus === 'success' && testResult && (
              <div className="flex items-center gap-2 text-sm text-success bg-success/10 px-3 py-2 rounded-md">
                <Check className="size-4" />
                <span>
                  Connected as <strong>@{testResult.username}</strong> · {testResult.projectCount} projects found
                </span>
              </div>
            )}
            
            {testStatus === 'error' && (
              <div className="flex items-center gap-2 text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-md">
                <AlertCircle className="size-4" />
                <span>Failed to connect. Check your URL and token.</span>
              </div>
            )}

            {/* Actions */}
            <div className="flex gap-3 pt-2">
              <Button
                variant="outline"
                className="flex-1"
                disabled={!isValidUrl || !isValidToken || testStatus === 'loading'}
                onClick={handleTestConnection}
              >
                {testStatus === 'loading' ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Testing...
                  </>
                ) : testStatus === 'success' ? (
                  <>
                    <Check className="size-4" />
                    Verified
                  </>
                ) : (
                  'Test Connection'
                )}
              </Button>
              <Button
                className="flex-1"
                disabled={testStatus !== 'success' || connectLoading}
                title={testStatus !== 'success' ? 'Test the connection first' : undefined}
                onClick={handleConnect}
              >
                {connectLoading ? (
                  <>
                    <Loader2 className="size-4 animate-spin" />
                    Connecting...
                  </>
                ) : (
                  'Connect'
                )}
              </Button>
            </div>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
