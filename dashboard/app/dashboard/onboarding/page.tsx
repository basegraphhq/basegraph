"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useToast } from "@/hooks/use-toast";

export default function OnboardingPage() {
	const router = useRouter();
	const { toast } = useToast();
	const [loading, setLoading] = useState(false);
	const [formData, setFormData] = useState({
		name: "",
		slug: "",
	});

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		setLoading(true);

		try {
			const payload: { name: string; slug?: string } = { name: formData.name };
			if (formData.slug.trim()) {
				payload.slug = formData.slug.trim();
			}

			const res = await fetch("/api/organization/create", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(payload),
			});

			if (!res.ok) {
				const error = await res.json();
				throw new Error(
					error.details || error.error || "Failed to create organization",
				);
			}

			toast({
				title: "Organization created",
				description: "You have successfully created your organization.",
			});

			router.push("/dashboard");
			router.refresh();
		} catch (error) {
			toast({
				title: "Error",
				description:
					error instanceof Error ? error.message : "Something went wrong",
				variant: "destructive",
			});
		} finally {
			setLoading(false);
		}
	};

	return (
		<div className="flex min-h-screen w-full items-center justify-center bg-muted/40 px-4">
			<Card className="w-full max-w-md shadow-lg">
				<CardHeader className="space-y-2">
					<CardTitle>Create Organization</CardTitle>
					<CardDescription>
						Create a new organization to get started with Relay.
					</CardDescription>
				</CardHeader>
				<form onSubmit={handleSubmit}>
					<CardContent className="space-y-6">
						<div className="space-y-2">
							<Label htmlFor="name">Organization Name</Label>
							<Input
								id="name"
								placeholder="Acme Corp"
								required
								value={formData.name}
								onChange={(e) =>
									setFormData((prev) => ({ ...prev, name: e.target.value }))
								}
							/>
						</div>
						<div className="space-y-2">
							<Label htmlFor="slug">Slug (Optional)</Label>
							<Input
								id="slug"
								placeholder="acme-corp"
								value={formData.slug}
								onChange={(e) =>
									setFormData((prev) => ({ ...prev, slug: e.target.value }))
								}
							/>
							<p className="mt-1 text-sm leading-5 text-muted-foreground">
								This will be used in your URL.
							</p>
						</div>
					</CardContent>
					<CardFooter className="pt-2">
						<Button className="w-full" type="submit" disabled={loading}>
							{loading ? "Creating..." : "Create Organization"}
						</Button>
					</CardFooter>
				</form>
			</Card>
		</div>
	);
}
