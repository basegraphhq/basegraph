import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

const SESSION_COOKIE = "relay_session";
const INVITE_TOKEN_COOKIE = "relay_invite_token";

type LogoutResponse = {
	message: string;
	logout_url?: string;
};

/**
 * Clears the local session and optionally gets the WorkOS logout URL.
 * @param returnTo - URL to redirect to after WorkOS logout (for full logout flow)
 * @returns The WorkOS logout URL if available, undefined otherwise
 */
async function clearSession(returnTo?: string): Promise<string | undefined> {
	const cookieStore = await cookies();
	const sessionId = cookieStore.get(SESSION_COOKIE)?.value;
	let workosLogoutUrl: string | undefined;

	if (sessionId) {
		try {
			const res = await fetch(`${RELAY_API_URL}/auth/logout-session`, {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({
					session_id: sessionId,
					return_to: returnTo,
				}),
			});
			if (res.ok) {
				const data: LogoutResponse = await res.json();
				workosLogoutUrl = data.logout_url;
			}
		} catch (error) {
			console.error("Error logging out from Relay:", error);
		}
	}

	cookieStore.delete(SESSION_COOKIE);
	cookieStore.delete(INVITE_TOKEN_COOKIE);

	return workosLogoutUrl;
}

// POST logout - clears session and returns WorkOS logout URL if available
export async function POST(request: NextRequest) {
	const body = await request.json().catch(() => ({}));
	const returnTo = body.return_to as string | undefined;
	const fullLogout = body.full_logout as boolean | undefined;

	const baseUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";
	const workosLogoutUrl = await clearSession(
		fullLogout ? returnTo || baseUrl : undefined
	);

	return NextResponse.json({
		message: "logged out",
		logout_url: workosLogoutUrl,
	});
}

// GET with redirect - used for full logout flows (e.g., email mismatch retry)
// Query params:
// - redirect: relative URL to redirect to after logout (default: /)
// - full: if "true", also logout from WorkOS
export async function GET(request: NextRequest) {
	const redirectTo = request.nextUrl.searchParams.get("redirect");
	const fullLogout = request.nextUrl.searchParams.get("full") === "true";
	const baseUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";

	// Calculate the final destination after all logouts
	let finalDestination = "/";
	if (redirectTo?.startsWith("/")) {
		finalDestination = redirectTo;
	}
	const finalUrl = new URL(finalDestination, baseUrl).toString();

	// Clear local session and get WorkOS logout URL if full logout requested
	const workosLogoutUrl = await clearSession(fullLogout ? finalUrl : undefined);

	// If full logout and we have a WorkOS logout URL, redirect to WorkOS first
	// WorkOS will then redirect back to our finalUrl
	if (fullLogout && workosLogoutUrl) {
		return NextResponse.redirect(workosLogoutUrl);
	}

	// Otherwise, redirect directly to the final destination
	return NextResponse.redirect(finalUrl);
}
