"use client";

import { ChevronDown, Terminal } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

type StatusEntry = {
	id: string;
	level: string;
	step: string;
	message: string;
	stamp: string;
	repo?: string;
};

type WorkspaceTerminalPeepProps = {
	enabled: boolean;
	streamUrl?: string;
};

const DEFAULT_STREAM_URL = "/api/agent-status/stream";
const MAX_LINES = 50;

export function WorkspaceTerminalPeep({
	enabled,
	streamUrl = DEFAULT_STREAM_URL,
}: WorkspaceTerminalPeepProps) {
	const [open, setOpen] = useState(false);
	const { lines, connected } = useStatusStream(enabled ? streamUrl : null);
	const seenIds = useRef(new Set<string>());
	const scrollRef = useRef<HTMLDivElement>(null);

	const isNewEntry = (id: string) => {
		if (seenIds.current.has(id)) return false;
		seenIds.current.add(id);
		return true;
	};

	const latestLine = lines[lines.length - 1];

	// Auto-scroll to bottom when new lines arrive or panel opens
	const lineCount = lines.length;
	// biome-ignore lint/correctness/useExhaustiveDependencies: lineCount triggers scroll on new entries
	useEffect(() => {
		if (open && scrollRef.current) {
			scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
		}
	}, [lineCount, open]);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<div className="rounded-lg border border-border/60 bg-card/40 overflow-hidden">
				<CollapsibleTrigger className="flex w-full items-center gap-3 px-4 py-2.5 text-left hover:bg-muted/40 transition-colors">
					<div className="flex items-center gap-2">
						<Terminal className="size-4 text-muted-foreground/70" />
						<span
							className={cn(
								"size-1.5 rounded-full",
								connected ? "bg-success" : "bg-muted-foreground/30",
							)}
						/>
					</div>
					<div className="flex-1 min-w-0">
						{latestLine ? (
							<p className="truncate text-xs text-muted-foreground font-mono">
								<span className="text-foreground/70">{latestLine.step}</span>
								<span className="mx-1.5 text-muted-foreground/50">·</span>
								<span>{latestLine.message}</span>
							</p>
						) : (
							<p className="text-xs text-muted-foreground">
								{enabled ? "Waiting for events..." : "Connect GitLab to see activity"}
							</p>
						)}
					</div>
					<ChevronDown
						className={cn(
							"size-4 text-muted-foreground transition-transform duration-200",
							open && "rotate-180",
						)}
					/>
				</CollapsibleTrigger>

				<CollapsibleContent>
					<div className="border-t border-border/40">
						<div
							ref={scrollRef}
							className="h-[160px] overflow-y-auto overscroll-contain"
						>
							<div className="p-3 font-mono text-xs space-y-1">
								{lines.length === 0 ? (
									<p className="text-muted-foreground py-4 text-center">
										No events yet
									</p>
								) : (
									lines.map((line) => (
										<LogLine
											key={line.id}
											entry={line}
											isNew={isNewEntry(line.id)}
										/>
									))
								)}
							</div>
						</div>
					</div>
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
}

function LogLine({
	entry,
	isNew,
}: {
	entry: StatusEntry;
	isNew: boolean;
}) {
	const levelStyles = {
		error: "text-destructive",
		warn: "text-amber-500",
		info: "text-foreground",
	}[entry.level] ?? "text-foreground";

	return (
		<div
			className={cn(
				"flex items-start gap-2 py-1 rounded px-2 -mx-2 hover:bg-muted/30 transition-colors",
				isNew && "animate-in fade-in-0 slide-in-from-bottom-1 duration-300",
			)}
		>
			<span className="text-muted-foreground/60 shrink-0 tabular-nums">
				{entry.stamp}
			</span>
			<span className="text-muted-foreground shrink-0 w-16 truncate">
				{entry.step}
			</span>
			<span className={cn("flex-1 min-w-0", levelStyles)}>
				{entry.message}
				{entry.repo && (
					<span className="ml-1 text-muted-foreground/60">({entry.repo})</span>
				)}
			</span>
		</div>
	);
}

// --- Hooks ---

function useStatusStream(url: string | null) {
	const [lines, setLines] = useState<StatusEntry[]>([]);
	const [connected, setConnected] = useState(false);

	useEffect(() => {
		if (!url) {
			setConnected(false);
			return;
		}

		const source = new EventSource(url);

		source.onopen = () => setConnected(true);
		source.onerror = () => setConnected(false);

		source.addEventListener("status", (event: MessageEvent) => {
			const entry = parseStatusEvent(event.data);
			if (entry) {
				setLines((prev) => [...prev, entry].slice(-MAX_LINES));
			}
		});

		return () => source.close();
	}, [url]);

	return { lines, connected };
}

// --- Parsing ---

function parseStatusEvent(raw: string): StatusEntry | null {
	try {
		const payload = JSON.parse(raw);
		const values =
			payload?.values ?? payload?.Values ?? payload?.data?.values ?? {};

		const message = values.message ?? values.event ?? "";
		if (!message) return null;

		const stamp = formatTimestamp(values.ts);
		const repo = values.repo ?? values.repo_slug ?? values.repo_id;

		return {
			id: `${payload?.id ?? payload?.ID ?? Date.now()}-${values.run_id ?? ""}-${values.step ?? ""}-${repo ?? ""}`,
			level: String(values.level ?? "info"),
			step: String(values.step ?? "event"),
			message: String(message),
			stamp,
			repo: repo ? String(repo) : undefined,
		};
	} catch {
		return null;
	}
}

function formatTimestamp(value?: string): string {
	const date = value ? new Date(value) : new Date();
	if (Number.isNaN(date.getTime())) {
		return new Date().toLocaleTimeString("en-US", { hour12: false });
	}
	return date.toLocaleTimeString("en-US", { hour12: false });
}
