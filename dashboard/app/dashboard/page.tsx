"use client";

import { AlertCircle, Gitlab, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { GitLabConnectPanel } from "@/components/gitlab-connect-panel";
import { GitlabRepoSelector } from "@/components/gitlab-repo-selector";
import { WorkspaceTerminalPeep } from "@/components/workspace-terminal-peep";
import { Typewriter } from "@/components/typewriter";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useToast } from "@/hooks/use-toast";
import { useSession } from "@/hooks/use-session";
import type {
	GitLabProject,
	GitLabSetupResponse,
	GitLabStatusResponse,
} from "@/lib/api";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const messages = [
	"Connect your tools to enable Relay's code analysis and context gathering.",
	"Then generate production-ready specs from your repositories in seconds.",
];

export default function DashboardPage() {
	const { data: session, isPending } = useSession();
	const router = useRouter();
	const { toast } = useToast();

	const [gitlabConnected, setGitlabConnected] = useState(false);
	const [gitlabStatus, setGitlabStatus] = useState<GitLabStatusResponse | null>(
		null,
	);
	const [gitlabError, setGitlabError] = useState<string | null>(null);
	const [refreshLoading, setRefreshLoading] = useState(false);
	const [projectsLoading, setProjectsLoading] = useState(false);
	const [projects, setProjects] = useState<GitLabProject[]>([]);
	const [selectedRepoIds, setSelectedRepoIds] = useState<number[]>([]);
	const [savingRepos, setSavingRepos] = useState(false);
	const [showCard, setShowCard] = useState(false);

	const selectionStorageKey = session?.user?.id
		? `relay_gitlab_repos_${session.user.id}`
		: "relay_gitlab_repos";

	useEffect(() => {
		if (!isPending && !session) {
			router.push("/");
		}
	}, [isPending, session, router]);

	useEffect(() => {
		if (!session?.user?.id) return;
		const stored = localStorage.getItem(selectionStorageKey);
		if (!stored) return;
		try {
			const parsed = JSON.parse(stored) as number[];
			if (Array.isArray(parsed)) {
				setSelectedRepoIds(parsed);
			}
		} catch {
			// ignore
		}
	}, [selectionStorageKey, session?.user?.id]);

	const handleGitlabConnect = async (data: GitLabSetupResponse) => {
		console.log(
			"GitLab connected:",
			data.integration_id,
			`${data.projects.length} projects`,
		);
		setGitlabError(null);
		setProjects(data.projects);
		setGitlabConnected(true);

		try {
			const status = await api.gitlab.status();
			setGitlabStatus(status);
			setGitlabConnected(status.connected);
		} catch {
			const synced = data.webhooks_created > 0 || data.repositories_added > 0;
			setGitlabConnected(true);
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

		if (data.projects.length > 0) {
			void loadProjects();
		}
	};

	const loadProjects = useCallback(async () => {
		setProjectsLoading(true);
		try {
			const [refreshed, enabled] = await Promise.all([
				api.gitlab.refresh(),
				api.gitlab.enabledRepos(),
			]);
			setProjects(refreshed.projects ?? []);
			setSelectedRepoIds(enabled.project_ids ?? []);
			localStorage.setItem(
				selectionStorageKey,
				JSON.stringify(enabled.project_ids ?? []),
			);
		} catch (err) {
			const message =
				err instanceof Error
					? err.message
					: "Failed to load repositories";
			setGitlabError(message);
		} finally {
			setProjectsLoading(false);
		}
	}, [selectionStorageKey]);

	useEffect(() => {
		api.gitlab
			.status()
			.then((status) => {
				setGitlabStatus(status);
				setGitlabConnected(status.connected);
				if (status.connected) {
					void loadProjects();
				}
			})
			.catch(() => {
				// keep silent; user can attempt connect
			});
	}, [loadProjects]);

	const handleGitlabRefresh = async () => {
		setRefreshLoading(true);
		setGitlabError(null);
		try {
			const [status, refreshed, enabled] = await Promise.all([
				api.gitlab.status(),
				api.gitlab.refresh(),
				api.gitlab.enabledRepos(),
			]);
			setGitlabStatus(status);
			setProjects(refreshed.projects ?? []);
			setSelectedRepoIds(enabled.project_ids ?? []);
			setGitlabConnected(status.connected);
		} catch (err) {
			const message =
				err instanceof Error ? err.message : "Failed to refresh GitLab sync";
			setGitlabError(message);
		} finally {
			setRefreshLoading(false);
		}
	};

	const handleToggleRepo = (projectId: number) => {
		setSelectedRepoIds((prev) =>
			prev.includes(projectId)
				? prev.filter((id) => id !== projectId)
				: [...prev, projectId],
		);
	};

	const handleSaveRepos = async () => {
		if (selectedRepoIds.length === 0) return;
		setSavingRepos(true);
		try {
			await api.gitlab.enableRepos(selectedRepoIds);
			localStorage.setItem(
				selectionStorageKey,
				JSON.stringify(selectedRepoIds),
			);
			toast({
				title: "Workspace setup queued",
				description: "Relay is preparing your repositories.",
			});
		} catch (err) {
			const message =
				err instanceof Error
					? err.message
					: "Failed to enable repositories";
			toast({
				title: "Failed to save selection",
				description: message,
				variant: "destructive",
			});
		} finally {
			setSavingRepos(false);
		}
	};

	const handleSelectAll = () => {
		setSelectedRepoIds(projects.map((project) => project.id));
	};

	const handleClearSelection = () => {
		setSelectedRepoIds([]);
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

			{/* Integration Cards */}
			<div
				className={cn(
					"transition-all duration-1000 ease-out",
					showCard ? "opacity-100 translate-y-0" : "opacity-0 translate-y-4",
				)}
			>
				<div className="space-y-6">
					<Card className="bg-card/50 shadow-none border-border/60 overflow-hidden">
						{/* GitLab connection header */}
						<div className="px-4 py-1 flex items-center gap-4">
							<div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-background text-foreground ring-1 ring-inset ring-border/50">
								<Gitlab className="size-[18px]" />
							</div>
							<div className="flex-1 min-w-0">
								<h4 className="text-sm font-medium leading-none text-foreground">
									Gitlab
								</h4>
								<p className="text-xs text-muted-foreground mt-0.5">
									{gitlabConnected
										? "Connected"
										: "Connect to enable code analysis"}
								</p>
							</div>

							{gitlabConnected ? (
								<div className="flex items-center gap-2">
									<span className="text-xs text-muted-foreground">
										{gitlabStatus?.status?.synced ? "Synced" : "Connected"}
									</span>
									<Button
										variant="ghost"
										size="icon"
										className="size-8 text-muted-foreground hover:text-foreground"
										onClick={handleGitlabRefresh}
										disabled={refreshLoading}
									>
										<RefreshCw
											className={cn(
												"size-4",
												refreshLoading && "animate-spin",
											)}
										/>
									</Button>
								</div>
							) : (
								<GitLabConnectPanel onConnect={handleGitlabConnect}>
									<Button
										size="sm"
										className="h-8 text-xs font-medium"
									>
										Connect
									</Button>
								</GitLabConnectPanel>
							)}
						</div>

						{/* Error states */}
						{gitlabError && (
							<div className="mx-4 mb-4 flex items-start gap-3 px-3 py-2.5 rounded-md bg-destructive/10 text-destructive text-sm">
								<AlertCircle className="size-4 mt-0.5 shrink-0" />
								<span>{gitlabError}</span>
							</div>
						)}
						{!gitlabError &&
							gitlabStatus?.status?.errors &&
							gitlabStatus.status.errors.length > 0 && (
								<div className="mx-4 mb-4 flex items-start gap-3 px-3 py-2.5 rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-500 text-sm">
									<AlertCircle className="size-4 mt-0.5 shrink-0" />
									<span>
										Partial sync: {gitlabStatus.status.errors.length} project
										{gitlabStatus.status.errors.length === 1 ? "" : "s"} failed
									</span>
								</div>
							)}

						{/* Repo selector - integrated into same card */}
						{gitlabConnected && (
							<div className="border-t border-border/40 px-4 py-3">
								<GitlabRepoSelector
									projects={projects}
									selectedIds={selectedRepoIds}
									loading={projectsLoading}
									saving={savingRepos}
									onToggle={handleToggleRepo}
									onSave={handleSaveRepos}
									onSelectAll={handleSelectAll}
									onClear={handleClearSelection}
								/>
							</div>
						)}
					</Card>

					{gitlabConnected && <WorkspaceTerminalPeep enabled={gitlabConnected} />}
				</div>

				<div className="mt-10 pt-6 border-t border-border/40">
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
