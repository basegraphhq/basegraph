import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import { ValidationStatus, validateSession } from "@/lib/auth";
import { RELAY_API_URL } from "@/lib/config";

const STATE_COOKIE = "relay_oauth_state";
const SESSION_COOKIE = "relay_session";
const INVITE_TOKEN_COOKIE = "relay_invite_token";
const SESSION_MAX_AGE = 7 * 24 * 60 * 60;

type ExchangeResponse = {
	user: {
		id: string;
		name: string;
		email: string;
		avatar_url?: string;
	};
	session_id: string;
	expires_in: number;
};

export async function GET(request: NextRequest) {
	const searchParams = request.nextUrl.searchParams;
	const code = searchParams.get("code");
	const state = searchParams.get("state");
	const error = searchParams.get("error");
	const errorDescription = searchParams.get("error_description");

	const baseUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";

	if (error) {
		console.error("OAuth error:", error, errorDescription);
		return NextResponse.redirect(new URL(`/?auth_error=${error}`, baseUrl));
	}

	const cookieStore = await cookies();
	const storedState = cookieStore.get(STATE_COOKIE)?.value;

	if (!state || state !== storedState) {
		console.error("State mismatch:", { expected: storedState, got: state });
		return NextResponse.redirect(
			new URL("/?auth_error=invalid_state", baseUrl),
		);
	}

	cookieStore.delete(STATE_COOKIE);

	// Get invite token (don't delete yet - need it for retry on failure)
	const inviteToken = cookieStore.get(INVITE_TOKEN_COOKIE)?.value;

	if (!code) {
		return NextResponse.redirect(new URL("/?auth_error=no_code", baseUrl));
	}

	try {
		const exchangeBody: { code: string; invite_token?: string } = { code };
		if (inviteToken) {
			exchangeBody.invite_token = inviteToken;
		}

		const res = await fetch(`${RELAY_API_URL}/auth/exchange`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify(exchangeBody),
		});

		if (!res.ok) {
			const errorData = await res.json().catch(() => ({}));
			console.error("Failed to exchange code:", res.status, errorData);

			const clearInviteCookie = (response: NextResponse) => {
				if (inviteToken) {
					response.cookies.delete(INVITE_TOKEN_COOKIE);
				}
				return response;
			};

			// Handle invite-specific errors
			if (errorData.code === "email_mismatch") {
				// On email mismatch, the backend keeps the session active (user IS logged in,
				// just with the wrong account). We set the session cookie so the logout flow
				// can find the session and get the WorkOS session ID for full logout.
				const redirectUrl = new URL("/invite", baseUrl);
				redirectUrl.searchParams.set("error", "email_mismatch");
				if (inviteToken) {
					redirectUrl.searchParams.set("token", inviteToken);
				}
				
				const response = NextResponse.redirect(redirectUrl);
				
				// Set session cookie if provided - needed for logout to work
				if (errorData.session_id) {
					response.cookies.set(SESSION_COOKIE, errorData.session_id, {
						httpOnly: true,
						secure: process.env.NODE_ENV === "production",
						sameSite: "lax",
						maxAge: SESSION_MAX_AGE,
						path: "/",
					});
				}
				
				return response;
			}
			if (errorData.code === "invite_expired") {
				return clearInviteCookie(
					NextResponse.redirect(new URL("/invite?error=expired", baseUrl)),
				);
			}
			if (errorData.code === "invite_used") {
				return clearInviteCookie(
					NextResponse.redirect(new URL("/invite?error=used", baseUrl)),
				);
			}
			if (errorData.code === "invite_revoked") {
				return clearInviteCookie(
					NextResponse.redirect(new URL("/invite?error=revoked", baseUrl)),
				);
			}
			if (errorData.code === "invite_only") {
				return clearInviteCookie(
					NextResponse.redirect(new URL("/?error=invite_only", baseUrl)),
				);
			}

			return clearInviteCookie(
				NextResponse.redirect(new URL("/?auth_error=exchange_failed", baseUrl)),
			);
		}

		const data: ExchangeResponse = await res.json();

		// Clear invite token only on success
		if (inviteToken) {
			cookieStore.delete(INVITE_TOKEN_COOKIE);
		}

		cookieStore.set(SESSION_COOKIE, data.session_id, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: SESSION_MAX_AGE,
			path: "/",
		});

		const result = await validateSession(data.session_id);
		// Default to dashboard - if validation fails, middleware will handle redirect
		const redirectPath =
			result.status === ValidationStatus.Valid &&
			result.data.has_organization === false
				? "/dashboard/onboarding"
				: "/dashboard";

		return NextResponse.redirect(new URL(redirectPath, baseUrl));
	} catch (error) {
		console.error("Error exchanging code:", error);
		return NextResponse.redirect(
			new URL("/?auth_error=callback_failed", baseUrl),
		);
	}
}
