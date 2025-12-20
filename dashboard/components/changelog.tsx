"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface LogEntry {
	date: string;
	title: string;
	description: string;
	tag?: string;
}

const changelogs: LogEntry[] = [
	{
		date: "Dec 4",
		title: "GitLab Integration",
		description:
			"Added GitLab authentication support. Connect your GitLab account to sync repositories and projects.",
		tag: "Feature",
	},
	{
		date: "Dec 4",
		title: "Design System Overhaul",
		description:
			"Implemented new design system with white page theme (Sierra). Refreshed UI components for a cleaner, more modern look.",
		tag: "Design",
	},
	{
		date: "Nov 30",
		title: "Dashboard Layout & Sidebar",
		description:
			"Redesigned dashboard layout with new sidebar navigation. Improved organization and easier access to features.",
		tag: "UI",
	},
	{
		date: "Nov 30",
		title: "Sync Card & Changelog",
		description:
			"Added sync status card and shipping logs panel. Stay updated with the latest changes and track your sync progress.",
		tag: "Feature",
	},
	{
		date: "Nov 29",
		title: "GitHub Authentication",
		description:
			"Added GitHub OAuth integration. Sign in with GitHub and automatically sync with waitlist upon login.",
		tag: "Feature",
	},
	{
		date: "Nov 27",
		title: "Google Authentication",
		description:
			"Added Google OAuth support. Multiple authentication options now available for seamless sign-in.",
		tag: "Feature",
	},
	{
		date: "Nov 26",
		title: "Dark Mode",
		description:
			"Introduced dark mode support. Toggle between light and dark themes for a comfortable viewing experience.",
		tag: "Feature",
	},
	{
		date: "Nov 26",
		title: "SEO Optimizations",
		description:
			"Enhanced SEO with improved metadata, sitemap, and performance optimizations. Better discoverability and search rankings.",
		tag: "Performance",
	},
	{
		date: "Nov 26",
		title: "Waitlist & Rate Limiting",
		description:
			"Added waitlist functionality with rate limiting. Secure and scalable user registration system.",
		tag: "Feature",
	},
];

export function Changelog({ className }: { className?: string }) {
	return (
		<aside
			className={cn(
				"flex flex-col h-full p-6 border-l border-border/40 bg-sidebar/30",
				className,
			)}
		>
			<div className="flex items-center justify-between mb-8">
				<h3 className="font-serif font-bold text-lg tracking-tight text-foreground">
					Shipping Logs
				</h3>
				<Badge
					variant="secondary"
					className="text-[10px] h-5 px-1.5 font-mono text-muted-foreground border-0 bg-muted/50"
				>
					v0.1.0
				</Badge>
			</div>

			<div className="flex-1 overflow-y-auto -mr-4 pr-4 custom-scrollbar">
				<div className="relative border-l border-border/40 ml-2 space-y-10 pb-6">
					{changelogs.map((log, index) => (
						<div key={index} className="relative pl-6 group">
							{/* Timeline dot */}
							<div
								className={cn(
									"absolute -left-[5px] top-1.5 h-2.5 w-2.5 rounded-full border-2 border-background transition-colors duration-300",
									index === 0
										? "bg-primary animate-pulse"
										: "bg-muted-foreground/20 group-hover:bg-muted-foreground/40",
								)}
							/>

							<div className="flex flex-col gap-1.5">
								<div className="flex items-center gap-2">
									<span className="text-xs font-mono text-muted-foreground/80">
										{log.date}
									</span>
									{log.tag && (
										<span
											className={cn(
												"text-[10px] px-1.5 py-0.5 rounded-full font-medium tracking-wide uppercase",
												log.tag === "Feature" &&
													"bg-blue-500/10 text-blue-600 dark:text-blue-400",
												log.tag === "Design" &&
													"bg-pink-500/10 text-pink-600 dark:text-pink-400",
												log.tag === "Performance" &&
													"bg-amber-500/10 text-amber-600 dark:text-amber-400",
												log.tag === "UI" &&
													"bg-purple-500/10 text-purple-600 dark:text-purple-400",
												log.tag === "Milestone" &&
													"bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
											)}
										>
											{log.tag}
										</span>
									)}
								</div>
								<h4 className="text-sm font-medium leading-none text-foreground/90">
									{log.title}
								</h4>
								<p className="text-sm text-muted-foreground leading-relaxed">
									{log.description}
								</p>
							</div>
						</div>
					))}
				</div>
			</div>
		</aside>
	);
}
