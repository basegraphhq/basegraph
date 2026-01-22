"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/theme-toggle";

type InviteStatus = 
	| "loading" 
	| "valid" 
	| "expired" 
	| "used" 
	| "revoked" 
	| "not_found" 
	| "email_mismatch" 
	| "already_signed_in_match"      // User is signed in with matching email
	| "already_signed_in_mismatch"   // User is signed in with different email
	| "error";

interface InviteData {
	email: string;
	expires_at: string;
}

interface CurrentUser {
	email: string;
	name: string;
}

function InviteContent() {
	const searchParams = useSearchParams();
	const router = useRouter();
	const token = searchParams.get("token");

	const [status, setStatus] = useState<InviteStatus>("loading");
	const [inviteData, setInviteData] = useState<InviteData | null>(null);
	const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);
	const [isRedirecting, setIsRedirecting] = useState(false);
	const [isAccepting, setIsAccepting] = useState(false);

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

		async function validateInviteAndCheckSession() {
			try {
				// Check if user is already signed in
				const meRes = await fetch("/api/auth/me", { credentials: "include" });
				const isSignedIn = meRes.ok;
				let user: CurrentUser | null = null;
				
				if (isSignedIn) {
					const meData = await meRes.json();
					user = { email: meData.email, name: meData.name };
					setCurrentUser(user);
				}

				// Validate the invite token
				const res = await fetch(`/api/invite/validate?token=${encodeURIComponent(token!)}`);
				const data = await res.json();

				if (res.ok) {
					setInviteData(data);
					
					// If we came back from email mismatch error, show that
					if (errorParam === "email_mismatch") {
						setStatus("email_mismatch");
					} else if (isSignedIn && user) {
						// User is already signed in - check if emails match
						const emailsMatch = user.email.toLowerCase() === data.email.toLowerCase();
						if (emailsMatch) {
							setStatus("already_signed_in_match");
						} else {
							setStatus("already_signed_in_mismatch");
						}
					} else {
						setStatus("valid");
					}
				} else {
					const code = data.code as string;
					if (data.email) {
						setInviteData({ email: data.email, expires_at: "" });
					}
					if (code === "expired") {
						setStatus("expired");
					} else if (code === "already_used") {
						// If invite is used and user is signed in with matching email, redirect to dashboard
						if (isSignedIn && user && data.email && user.email.toLowerCase() === data.email.toLowerCase()) {
							router.push("/dashboard");
							return;
						}
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

		validateInviteAndCheckSession();
	}, [token, searchParams, router]);

	const handleContinue = () => {
		if (!token) return;
		setIsRedirecting(true);
		
		let loginUrl = `/api/auth/login?invite_token=${encodeURIComponent(token)}`;
		// Pass expected email as login_hint to pre-fill auth flow
		if (inviteData?.email) {
			loginUrl += `&login_hint=${encodeURIComponent(inviteData.email)}`;
		}
		window.location.href = loginUrl;
	};

	const handleLogout = () => {
		if (!token) return;
		setIsRedirecting(true);
		
		// Build the invite URL to return to after logout
		const inviteUrl = `/invite?token=${encodeURIComponent(token)}`;
		
		// Do a full logout (including WorkOS) then redirect back to invite page
		// User will then see the normal "Continue with Sign In" flow
		window.location.href = `/api/auth/logout?full=true&redirect=${encodeURIComponent(inviteUrl)}`;
	};

	const handleAcceptWhileSignedIn = async () => {
		if (!token) return;
		setIsAccepting(true);
		
		try {
			const res = await fetch("/api/invite/accept", {
				method: "POST",
				credentials: "include",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ token }),
			});
			
			if (res.ok) {
				router.push("/dashboard");
			} else {
				const data = await res.json();
				if (data.code === "email_mismatch") {
					setStatus("already_signed_in_mismatch");
				} else {
					setStatus("error");
				}
			}
		} catch {
			setStatus("error");
		} finally {
			setIsAccepting(false);
		}
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
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Invitation Already Used</h1>
							<p className="text-muted-foreground">
								{inviteData?.email ? (
									<>
										The invitation for{" "}
										<span className="font-medium text-foreground">{inviteData.email}</span>{" "}
										has already been used. If this is your account, sign in below.
									</>
								) : (
									"This invitation has already been used. If you have an account, sign in below."
								)}
							</p>
						</div>

						<Button
							size="lg"
							onClick={() => {
								const loginUrl = inviteData?.email 
									? `/api/auth/login?login_hint=${encodeURIComponent(inviteData.email)}`
									: `/api/auth/login`;
								window.location.href = loginUrl;
							}}
							className="w-full"
						>
							Sign In
						</Button>

						<Button variant="link" onClick={() => router.push("/")}>
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

				{status === "already_signed_in_match" && inviteData && currentUser && (
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Welcome to Relay</h1>
							<p className="text-muted-foreground">
								You're signed in as{" "}
								<span className="font-medium text-foreground">{currentUser.email}</span>.
								<br />
								Click below to accept your invitation.
							</p>
						</div>

						<Button
							size="lg"
							onClick={handleAcceptWhileSignedIn}
							disabled={isAccepting}
							className="w-full"
						>
							{isAccepting ? "Accepting..." : "Accept Invitation"}
						</Button>
					</div>
				)}

				{status === "already_signed_in_mismatch" && inviteData && currentUser && (
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Wrong Account</h1>
							<p className="text-muted-foreground">
								You're signed in as{" "}
								<span className="font-medium text-foreground">{currentUser.email}</span>,
								but this invitation is for{" "}
								<span className="font-medium text-foreground">{inviteData.email}</span>.
							</p>
						</div>

						{token && (
							<Button
								size="lg"
								onClick={handleLogout}
								disabled={isRedirecting}
								className="w-full"
							>
								{isRedirecting ? "Signing out..." : "Logout & Switch Account"}
							</Button>
						)}

						<p className="text-xs text-muted-foreground">
							After logging out, sign in with {inviteData.email} to accept this invitation.
						</p>
					</div>
				)}

				{status === "email_mismatch" && (
					<div className="text-center space-y-6">
						<div>
							<h1 className="text-2xl font-semibold mb-2">Wrong Account</h1>
							<p className="text-muted-foreground">
								{inviteData ? (
									<>
										This invite is for{" "}
										<span className="font-medium text-foreground">{inviteData.email}</span>.
										<br />
										You signed in with a different account.
									</>
								) : (
									"You signed in with an account that doesn't match this invitation."
								)}
							</p>
						</div>

						{token && (
							<Button
								size="lg"
								onClick={handleLogout}
								disabled={isRedirecting}
								className="w-full"
							>
								{isRedirecting ? "Signing out..." : "Logout & Try Again"}
							</Button>
						)}

						<p className="text-xs text-muted-foreground">
							After logging out, sign in with the correct account.
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
