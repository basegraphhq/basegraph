"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useToast } from "@/hooks/use-toast";
import { cn } from "@/lib/utils";

const ISSUE_TRACKERS = ["Linear", "Github Issues", "Jira", "Gitlab Issues", "Other"] as const;
const CODE_HOSTS = ["Github", "Gitlab", "Bitbucket", "Other"] as const;

type IssueTracker = (typeof ISSUE_TRACKERS)[number];
type CodeHost = (typeof CODE_HOSTS)[number];

interface FormData {
	name: string;
	email: string;
	company: string;
	issueTracker: IssueTracker | "";
	issueTrackerOther: string;
	codeHost: CodeHost | "";
	codeHostOther: string;
	message: string;
}

const initialFormData: FormData = {
	name: "",
	email: "",
	company: "",
	issueTracker: "",
	issueTrackerOther: "",
	codeHost: "",
	codeHostOther: "",
	message: "",
};

function SelectChip({
	label,
	selected,
	onClick,
	disabled,
}: {
	label: string;
	selected: boolean;
	onClick: () => void;
	disabled?: boolean;
}) {
	return (
		<button
			type="button"
			onClick={onClick}
			disabled={disabled}
			className={cn(
				"px-3 py-1.5 text-sm rounded-md border transition-all",
				"focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
				selected
					? "bg-foreground text-background border-foreground"
					: "bg-transparent text-muted-foreground border-border hover:border-foreground/50 hover:text-foreground",
				disabled && "opacity-50 cursor-not-allowed",
			)}
		>
			{label}
		</button>
	);
}

export function ContactDialog() {
	const [open, setOpen] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [formData, setFormData] = useState<FormData>(initialFormData);
	const [submitted, setSubmitted] = useState(false);
	const [submittedData, setSubmittedData] = useState<{ name: string; email: string } | null>(null);
	const { toast } = useToast();

	const handleOpenChange = (isOpen: boolean) => {
		setOpen(isOpen);
		if (!isOpen) {
			// Reset state when dialog closes
			setTimeout(() => {
				setSubmitted(false);
				setSubmittedData(null);
				setFormData(initialFormData);
			}, 150); // Small delay so animation completes
		}
	};

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();

		if (!formData.name.trim()) {
			toast({
				title: "Name required",
				description: "Please enter your name.",
				variant: "destructive",
			});
			return;
		}

		if (!formData.email || !formData.email.includes("@")) {
			toast({
				title: "Invalid email",
				description: "Please enter a valid email address.",
				variant: "destructive",
			});
			return;
		}

		if (!formData.company.trim()) {
			toast({
				title: "Company required",
				description: "Please enter your company name.",
				variant: "destructive",
			});
			return;
		}

		if (!formData.issueTracker) {
			toast({
				title: "Issue tracker required",
				description: "Please select your issue tracker.",
				variant: "destructive",
			});
			return;
		}

		if (formData.issueTracker === "Other" && !formData.issueTrackerOther.trim()) {
			toast({
				title: "Please specify",
				description: "Tell us which issue tracker you use.",
				variant: "destructive",
			});
			return;
		}

		if (!formData.codeHost) {
			toast({
				title: "Code host required",
				description: "Please select where your code is hosted.",
				variant: "destructive",
			});
			return;
		}

		if (formData.codeHost === "Other" && !formData.codeHostOther.trim()) {
			toast({
				title: "Please specify",
				description: "Tell us where your code is hosted.",
				variant: "destructive",
			});
			return;
		}

		setIsLoading(true);

		try {
			const response = await fetch("/api/contact", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					name: formData.name.trim(),
					email: formData.email.trim(),
					company: formData.company.trim(),
					issueTracker: formData.issueTracker === "Other" 
						? formData.issueTrackerOther.trim() 
						: formData.issueTracker,
					codeHost: formData.codeHost === "Other"
						? formData.codeHostOther.trim()
						: formData.codeHost,
					message: formData.message.trim(),
				}),
			});

			const data = await response.json();

			if (!response.ok) {
				if (response.status === 429) {
					const retryAfter = data.retryAfter || 60;
					throw new Error(
						`Too many requests. Please try again in ${Math.ceil(retryAfter / 60)} minute${retryAfter > 60 ? "s" : ""}.`,
					);
				}
				throw new Error(data.error || "Failed to submit request");
			}

			setSubmittedData({ name: formData.name.trim(), email: formData.email.trim() });
			setSubmitted(true);
		} catch (error) {
			toast({
				title: "Error",
				description:
					error instanceof Error
						? error.message
						: "Something went wrong. Please try again.",
				variant: "destructive",
			});
		} finally {
			setIsLoading(false);
		}
	};

	const handleInputChange = (
		e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
	) => {
		setFormData((prev) => ({
			...prev,
			[e.target.name]: e.target.value,
		}));
	};

	const selectIssueTracker = (value: IssueTracker) => {
		setFormData((prev) => ({
			...prev,
			issueTracker: value,
			issueTrackerOther: value === "Other" ? prev.issueTrackerOther : "",
		}));
	};

	const selectCodeHost = (value: CodeHost) => {
		setFormData((prev) => ({
			...prev,
			codeHost: value,
			codeHostOther: value === "Other" ? prev.codeHostOther : "",
		}));
	};

	return (
		<Dialog open={open} onOpenChange={handleOpenChange}>
			<DialogTrigger asChild>
				<Button size="lg">Request Access</Button>
			</DialogTrigger>
			<DialogContent className="sm:max-w-md max-h-[90vh] overflow-y-auto">
				{submitted && submittedData ? (
					<div className="py-8 text-center space-y-6">
						<div className="flex justify-center">
							<div className="w-12 h-12 rounded-full bg-foreground/10 flex items-center justify-center animate-in fade-in zoom-in-50 duration-300">
								<svg
									className="w-6 h-6 text-foreground"
									fill="none"
									viewBox="0 0 24 24"
									stroke="currentColor"
									strokeWidth={2}
									aria-hidden="true"
								>
									<path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
								</svg>
							</div>
						</div>
						<div className="space-y-2 animate-in fade-in slide-in-from-bottom-2 duration-300 delay-100">
							<h3 className="text-lg font-semibold">
								You're on the list, {submittedData.name.split(" ")[0]}
							</h3>
							<p className="text-muted-foreground text-sm max-w-[280px] mx-auto">
								We review every request personally. Expect to hear from a founder within an hour or two.
							</p>
						</div>
						<Button
							onClick={() => handleOpenChange(false)}
							variant="outline"
							className="animate-in fade-in duration-300 delay-200"
						>
							Got it
						</Button>
					</div>
				) : (
					<>
						<DialogHeader>
							<DialogTitle>Request Access</DialogTitle>
							<DialogDescription>
								We're onboarding teams in batches. Tell us a bit about yourself and
								we'll be in touch.
							</DialogDescription>
						</DialogHeader>
						<form onSubmit={handleSubmit} className="space-y-4">
					{/* Basic Info */}
					<div className="space-y-2">
						<Label htmlFor="name">Name</Label>
						<Input
							id="name"
							name="name"
							placeholder="Your name"
							value={formData.name}
							onChange={handleInputChange}
							disabled={isLoading}
							required
						/>
					</div>
					<div className="space-y-2">
						<Label htmlFor="email">Email</Label>
						<Input
							id="email"
							name="email"
							type="email"
							placeholder="Work email preferred"
							value={formData.email}
							onChange={handleInputChange}
							disabled={isLoading}
							required
						/>
					</div>
					<div className="space-y-2">
						<Label htmlFor="company">Company</Label>
						<Input
							id="company"
							name="company"
							placeholder="Your company"
							value={formData.company}
							onChange={handleInputChange}
							disabled={isLoading}
							required
						/>
					</div>

					{/* Stack Questions */}
					<div className="pt-2 border-t border-border/50 space-y-4">
						<div className="space-y-2">
							<Label>What does your team use for tracking issues?</Label>
							<div className="flex flex-wrap gap-2">
								{ISSUE_TRACKERS.map((tracker) => (
									<SelectChip
										key={tracker}
										label={tracker}
										selected={formData.issueTracker === tracker}
										onClick={() => selectIssueTracker(tracker)}
										disabled={isLoading}
									/>
								))}
							</div>
							{formData.issueTracker === "Other" && (
								<Input
									name="issueTrackerOther"
									placeholder="Which issue tracker?"
									value={formData.issueTrackerOther}
									onChange={handleInputChange}
									disabled={isLoading}
									className="mt-2"
								/>
							)}
						</div>

						<div className="space-y-2">
							<Label>Where is your code hosted?</Label>
							<div className="flex flex-wrap gap-2">
								{CODE_HOSTS.map((host) => (
									<SelectChip
										key={host}
										label={host}
										selected={formData.codeHost === host}
										onClick={() => selectCodeHost(host)}
										disabled={isLoading}
									/>
								))}
							</div>
							{formData.codeHost === "Other" && (
								<Input
									name="codeHostOther"
									placeholder="Which platform?"
									value={formData.codeHostOther}
									onChange={handleInputChange}
									disabled={isLoading}
									className="mt-2"
								/>
							)}
						</div>
					</div>

					{/* Message */}
					<div className="pt-2 border-t border-border/50 space-y-2">
						<Label htmlFor="message">
							Anything else? <span className="text-muted-foreground">(optional)</span>
						</Label>
						<Textarea
							id="message"
							name="message"
							placeholder="What do your agents keep missing that feels obvious to your team?"
							value={formData.message}
							onChange={handleInputChange}
							disabled={isLoading}
							rows={3}
						/>
					</div>

						<Button type="submit" disabled={isLoading} className="w-full">
						{isLoading ? "Submitting..." : "Submit"}
					</Button>
				</form>
					</>
				)}
			</DialogContent>
		</Dialog>
	);
}
