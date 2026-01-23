"use client";

import { Check, ChevronDown } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "@/components/ui/command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover";
import type { GitLabProject } from "@/lib/api";
import { cn } from "@/lib/utils";

type GitlabRepoSelectorProps = {
	projects: GitLabProject[];
	selectedIds: number[];
	loading?: boolean;
	saving?: boolean;
	onToggle: (projectId: number) => void;
	onSave: () => void;
	onSelectAll: () => void;
	onClear: () => void;
};

export function GitlabRepoSelector({
	projects,
	selectedIds,
	loading,
	saving,
	onToggle,
	onSave,
	onSelectAll,
	onClear,
}: GitlabRepoSelectorProps) {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");

	const filtered = useMemo(() => {
		const trimmed = search.trim().toLowerCase();
		if (!trimmed) return projects;
		return projects.filter((project) =>
			project.path_with_namespace.toLowerCase().includes(trimmed),
		);
	}, [projects, search]);

	const selectedProjects = projects.filter((p) => selectedIds.includes(p.id));

	return (
		<div className="space-y-4">
			{/* Label */}
			<p className="text-sm text-muted-foreground">
				{selectedIds.length === 0
					? "Select repositories to index"
					: `${selectedIds.length} ${selectedIds.length === 1 ? "repository" : "repositories"} selected`}
			</p>

			{/* Combobox */}
			<Popover open={open} onOpenChange={setOpen}>
				<PopoverTrigger asChild>
					<button
						type="button"
						disabled={loading || projects.length === 0}
						className={cn(
							"flex h-9 w-full items-center justify-between rounded-md border border-input bg-background px-3 text-sm shadow-sm transition-colors",
							"hover:bg-muted focus:outline-none focus:ring-1 focus:ring-ring",
							"disabled:cursor-not-allowed disabled:opacity-50",
						)}
					>
						<span className={cn(
							"truncate",
							selectedIds.length === 0 && "text-muted-foreground"
						)}>
							{loading
								? "Loading..."
								: selectedIds.length === 0
									? "Choose repositories..."
									: selectedProjects
											.slice(0, 3)
											.map((p) => p.path_with_namespace.split("/").pop())
											.join(", ") +
										(selectedProjects.length > 3
											? ` +${selectedProjects.length - 3}`
											: "")}
						</span>
						<ChevronDown className="size-4 shrink-0 opacity-50" />
					</button>
				</PopoverTrigger>

				<PopoverContent
					className="w-(--radix-popover-trigger-width) p-0"
					align="start"
				>
					<Command shouldFilter={false}>
						<CommandInput
							placeholder="Search..."
							value={search}
							onValueChange={setSearch}
						/>
						<CommandList className="max-h-[200px]">
							<CommandEmpty className="py-4 text-center text-sm text-muted-foreground">
								No repositories found
							</CommandEmpty>
							<CommandGroup>
								{filtered.map((project) => {
									const isSelected = selectedIds.includes(project.id);
									const name = project.path_with_namespace.split("/").pop();
									return (
										<CommandItem
											key={project.id}
											value={project.path_with_namespace}
											onSelect={() => onToggle(project.id)}
											className="cursor-pointer py-2 data-[selected=true]:bg-muted data-[selected=true]:text-foreground"
										>
											<div
												className={cn(
													"mr-2.5 flex size-4 shrink-0 items-center justify-center rounded border transition-colors",
													isSelected
														? "border-foreground bg-foreground text-background"
														: "border-border",
												)}
											>
												{isSelected && <Check className="size-2.5" strokeWidth={3} />}
											</div>
											<span className="flex-1 truncate">{name}</span>
										</CommandItem>
									);
								})}
							</CommandGroup>
						</CommandList>

						{/* Footer */}
						<div className="flex items-center justify-between border-t px-2 py-1.5">
							<div className="flex gap-1">
								<button
									type="button"
									onClick={onSelectAll}
									disabled={projects.length === 0}
									className="rounded px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
								>
									All
								</button>
								<button
									type="button"
									onClick={onClear}
									disabled={selectedIds.length === 0}
									className="rounded px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
								>
									None
								</button>
							</div>
							<span className="text-xs tabular-nums text-muted-foreground">
								{selectedIds.length}/{projects.length}
							</span>
						</div>
					</Command>
				</PopoverContent>
			</Popover>

			{/* Selected list (when overflow) */}
			{selectedProjects.length > 3 && (
				<p className="text-xs text-muted-foreground">
					{selectedProjects.map((p) => p.path_with_namespace.split("/").pop()).join(", ")}
				</p>
			)}

			{/* Action */}
			{selectedIds.length > 0 && (
				<div className="flex justify-end">
					<Button size="sm" onClick={onSave} disabled={saving}>
						{saving ? "Syncing..." : "Sync"}
					</Button>
				</div>
			)}
		</div>
	);
}
