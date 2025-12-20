"use client";

import { AlertCircle, Check, Gitlab, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { GitLabConnectPanel } from "@/components/gitlab-connect-panel";
import { Typewriter } from "@/components/typewriter";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useSession } from "@/hooks/use-session";
import type { GitLabSetupResponse, GitLabStatusResponse } from "@/lib/api";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const messages = [
	"Connect your tools to enable Relay's code analysis and context gathering.",
	"Then generate production-ready specs from your repositories in seconds.",
];

export default function DashboardPage() {
	const { data: session, isPending } = useSession();
	const router = useRouter();

	const [gitlabConnected, setGitlabConnected] = useState(false);
	const [gitlabStatus, setGitlabStatus] = useState<GitLabStatusResponse | null>(
		null,
	);
	const [gitlabError, setGitlabError] = useState<string | null>(null);
	const [refreshLoading, setRefreshLoading] = useState(false);
	const [showCard, setShowCard] = useState(false);

	useEffect(() => {
		if (!isPending && !session) {
			router.push("/");
		}
	}, [isPending, session, router]);

	useEffect(() => {
		api.gitlab
			.status()
			.then((status) => {
				setGitlabStatus(status);
				const synced = status.status?.synced ?? status.repos_count > 0;
				setGitlabConnected(synced);
			})
			.catch(() => {
				// keep silent; user can attempt connect
			});
	}, []);

	const handleGitlabConnect = async (data: GitLabSetupResponse) => {
		console.log(
			"GitLab connected:",
			data.integration_id,
			`${data.projects.length} projects`,
		);
		setGitlabError(null);

		try {
			const status = await api.gitlab.status();
			setGitlabStatus(status);
			setGitlabConnected(
				status.connected && (status.status?.synced || status.repos_count > 0),
			);
		} catch {
			const synced = data.webhooks_created > 0 || data.repositories_added > 0;
			setGitlabConnected(synced);
			setGitlabStatus({
				connected: true,
				integration_id: data.integration_id,
				status: {
					synced,
					webhooks_created: data.webhooks_created,
					repositories_added: data.repositories_added,
					errors: data.errors,
				},
				repos_count: data.repositories_added,
			});
		}
	};

	const handleGitlabRefresh = async () => {
		setRefreshLoading(true);
		setGitlabError(null);
		try {
			await api.gitlab.refresh();
			const status = await api.gitlab.status();
			setGitlabStatus(status);
			setGitlabConnected(
				status.connected && (status.status?.synced || status.repos_count > 0),
			);
		} catch (err) {
			const message =
				err instanceof Error ? err.message : "Failed to refresh GitLab sync";
			setGitlabError(message);
		} finally {
			setRefreshLoading(false);
		}
	};

	// Loading state - show nothing while session loads (fast enough to not need skeleton)
	if (isPending) {
		return null;
	}

	// Not authenticated (will redirect)
	if (!session) {
		return null;
	}

	return (
		<div className="content-spacing max-w-2xl mx-auto">
			{/* Greeting Section */}
			<div className="mb-10 pt-6">
				<Typewriter
					messages={[`Hey ${session.user?.name?.split(" ")[0]}!`]}
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
			<div
				className={cn(
					"transition-all duration-1000 ease-out",
					showCard ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4",
				)}
			>
				<Card className="bg-card/50 shadow-none border-border/60 p-2 gap-1">
					{/* GitLab Integration */}
					<div className="interactive-row">
						<div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-background text-foreground ring-1 ring-inset ring-border/50">
							<Gitlab className="size-5" />
						</div>
						<div className="flex-1 min-w-0 grid gap-0.5">
							<h4 className="text-sm font-medium leading-none text-foreground">
								Sync with GitLab
							</h4>
							<p className="text-sm text-muted-foreground leading-normal">
								{gitlabConnected && gitlabStatus?.repos_count
									? `${gitlabStatus.repos_count} ${gitlabStatus.repos_count === 1 ? "repository" : "repositories"} synced`
									: "Connect your GitLab repositories to enable code analysis and codebase mapping"}
							</p>
						</div>

						{gitlabConnected ? (
							<div className="flex items-center gap-2">
								<Button
									disabled
									variant="outline"
									size="sm"
									className="min-w-[90px] h-8 text-xs font-medium state-connected"
								>
									<Check className="size-3 mr-1.5" />
									{gitlabStatus?.status?.synced ? "Synced" : "Connected"}
								</Button>
								<Button
									variant="outline"
									size="sm"
									className="min-w-[90px] h-8 text-xs font-medium"
									onClick={handleGitlabRefresh}
									disabled={refreshLoading}
								>
									{refreshLoading ? (
										<>
											<RefreshCw className="size-3 mr-1.5 animate-spin" />
											Syncing
										</>
									) : (
										"Refresh"
									)}
								</Button>
							</div>
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

					{/* Error / Warning Messages */}
					{gitlabError && (
						<div className="flex items-start gap-3 px-3 py-2.5 rounded-md bg-destructive/10 text-destructive text-sm">
							<AlertCircle className="size-4 mt-0.5 shrink-0" />
							<span>{gitlabError}</span>
						</div>
					)}
					{!gitlabError &&
						gitlabStatus?.status?.errors &&
						gitlabStatus.status.errors.length > 0 && (
							<div className="flex items-start gap-3 px-3 py-2.5 rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-500 text-sm">
								<AlertCircle className="size-4 mt-0.5 shrink-0" />
								<div>
									<span className="font-medium">Partial sync completed.</span>
									<span className="ml-1">
										{gitlabStatus.status.errors.length}{" "}
										{gitlabStatus.status.errors.length === 1
											? "project"
											: "projects"}{" "}
										failed to sync.
									</span>
								</div>
							</div>
						)}
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
	);
}
