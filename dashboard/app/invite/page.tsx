"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";

type InviteStatus = "loading" | "valid" | "expired" | "used" | "revoked" | "not_found" | "email_mismatch" | "error";

interface InviteData {
	email: string;
	expires_at: string;
}

function InviteContent() {
	const searchParams = useSearchParams();
	const router = useRouter();
	const token = searchParams.get("token");

	const [status, setStatus] = useState<InviteStatus>("loading");
	const [inviteData, setInviteData] = useState<InviteData | null>(null);
	const [isRedirecting, setIsRedirecting] = useState(false);

	useEffect(() => {
		const errorParam = searchParams.get("error");
		
		// Handle non-token errors immediately
		if (errorParam && errorParam !== "email_mismatch") {
			if (errorParam === "expired") {
				setStatus("expired");
			} else if (errorParam === "used") {
				setStatus("used");
			} else if (errorParam === "revoked") {
				setStatus("revoked");
			} else {
				setStatus("error");
			}
			return;
		}

		if (!token) {
			setStatus("not_found");
			return;
		}

		async function validateInvite() {
			try {
				const res = await fetch(`/api/invite/validate?token=${encodeURIComponent(token!)}`);
				const data = await res.json();

				if (res.ok) {
					setInviteData(data);
					// If we came back from email mismatch, show that error with the email
					if (errorParam === "email_mismatch") {
						setStatus("email_mismatch");
					} else {
						setStatus("valid");
					}
				} else {
					const code = data.code as string;
					if (code === "expired") {
						setStatus("expired");
					} else if (code === "already_used") {
						setStatus("used");
					} else if (code === "revoked") {
						setStatus("revoked");
					} else {
						setStatus("not_found");
					}
				}
			} catch {
				setStatus("error");
			}
		}

		validateInvite();
	}, [token, searchParams]);

	const handleContinue = async () => {
		if (!token) return;
		setIsRedirecting(true);
		// Store token in cookie and redirect to auth
		router.push(`/api/auth/login?invite_token=${encodeURIComponent(token)}`);
	};

	return (
		<main className="relative min-h-screen bg-background text-foreground flex items-center justify-center">
			<ThemeToggle />

			{/* Background decoration */}
			<div className="absolute inset-0 pointer-events-none overflow-hidden">
				<div className="absolute top-0 left-1/4 w-[600px] h-[600px] bg-gradient-to-br from-primary/5 to-transparent rounded-full blur-3xl" />
				<div className="absolute bottom-1/4 right-0 w-[500px] h-[500px] bg-gradient-to-tl from-accent/5 to-transparent rounded-full blur-3xl" />
			</div>

			<div className="relative z-10 w-full max-w-md mx-4">
				{status === "loading" && (
					<div className="text-center">
						<div className="animate-pulse">
							<div className="h-8 w-48 bg-muted rounded mx-auto mb-4" />
							<div className="h-4 w-64 bg-muted rounded mx-auto" />
						</div>
					</div>
				)}

				{status === "valid" && inviteData && (
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Welcome to Relay</h1>
							<p className="text-muted-foreground">
								You've been invited to join Relay. Sign in with{" "}
								<span className="font-medium text-foreground">{inviteData.email}</span>{" "}
								to continue.
							</p>
						</div>

						<Button
							size="lg"
							onClick={handleContinue}
							disabled={isRedirecting}
							className="w-full"
						>
							{isRedirecting ? "Redirecting..." : "Continue with Sign In"}
						</Button>

						<p className="text-xs text-muted-foreground">
							Make sure to sign in with the same email address the invitation was sent to.
						</p>
					</div>
				)}

				{status === "expired" && (
					<div className="text-center space-y-4">
						<h1 className="text-2xl font-semibold">Invitation Expired</h1>
						<p className="text-muted-foreground">
							This invitation has expired. Please contact the person who invited you to
							request a new invitation.
						</p>
						<Button variant="outline" onClick={() => router.push("/")}>
							Go to Homepage
						</Button>
					</div>
				)}

				{status === "used" && (
					<div className="text-center space-y-4">
						<h1 className="text-2xl font-semibold">Invitation Already Used</h1>
						<p className="text-muted-foreground">
							This invitation has already been used. If you already have an account, you
							can sign in directly.
						</p>
						<Button variant="outline" onClick={() => router.push("/")}>
							Go to Homepage
						</Button>
					</div>
				)}

				{status === "revoked" && (
					<div className="text-center space-y-4">
						<h1 className="text-2xl font-semibold">Invitation Revoked</h1>
						<p className="text-muted-foreground">
							This invitation has been revoked. Please contact the person who invited you
							for assistance.
						</p>
						<Button variant="outline" onClick={() => router.push("/")}>
							Go to Homepage
						</Button>
					</div>
				)}

				{status === "email_mismatch" && (
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Wrong Account</h1>
							<p className="text-muted-foreground">
								The email you signed in with doesn't match the invitation.
								{inviteData && (
									<>
										{" "}This invite was sent to{" "}
										<span className="font-medium text-foreground">{inviteData.email}</span>.
									</>
								)}
							</p>
						</div>

						{token && (
							<Button
								size="lg"
								onClick={handleContinue}
								disabled={isRedirecting}
								className="w-full"
							>
								{isRedirecting ? "Redirecting..." : "Try Again with Correct Account"}
							</Button>
						)}

						<p className="text-xs text-muted-foreground">
							Make sure to sign in with the same email address the invitation was sent to.
						</p>
					</div>
				)}

				{(status === "not_found" || status === "error") && (
					<div className="text-center space-y-4">
						<h1 className="text-2xl font-semibold">Invalid Invitation</h1>
						<p className="text-muted-foreground">
							This invitation link is invalid or has been removed. Please check the link
							or contact support.
						</p>
						<Button variant="outline" onClick={() => router.push("/")}>
							Go to Homepage
						</Button>
					</div>
				)}
			</div>
		</main>
	);
}

function LoadingState() {
	return (
		<main className="relative min-h-screen bg-background text-foreground flex items-center justify-center">
			<div className="text-center">
				<div className="animate-pulse">
					<div className="h-8 w-48 bg-muted rounded mx-auto mb-4" />
					<div className="h-4 w-64 bg-muted rounded mx-auto" />
				</div>
			</div>
		</main>
	);
}

export default function InvitePage() {
	return (
		<Suspense fallback={<LoadingState />}>
			<InviteContent />
		</Suspense>
	);
}
