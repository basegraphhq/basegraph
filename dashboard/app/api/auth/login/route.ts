import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

const STATE_COOKIE = "relay_oauth_state";
const INVITE_TOKEN_COOKIE = "relay_invite_token";
const STATE_MAX_AGE = 600;

type AuthURLResponse = {
	authorization_url: string;
	state: string;
};

export async function GET(request: NextRequest) {
	const inviteToken = request.nextUrl.searchParams.get("invite_token");
	const baseUrl = process.env.NEXT_PUBLIC_APP_URL || "http://localhost:3000";

	try {
		const res = await fetch(`${RELAY_API_URL}/auth/url`);

		if (!res.ok) {
			console.error("Failed to get auth URL from Relay:", res.status);
			return NextResponse.redirect(
				new URL("/?auth_error=relay_error", baseUrl),
			);
		}

		const data: AuthURLResponse = await res.json();

		const cookieStore = await cookies();
		cookieStore.set(STATE_COOKIE, data.state, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: STATE_MAX_AGE,
			path: "/",
		});

		// Store invite token if present
		if (inviteToken) {
			cookieStore.set(INVITE_TOKEN_COOKIE, inviteToken, {
				httpOnly: true,
				secure: process.env.NODE_ENV === "production",
				sameSite: "lax",
				maxAge: STATE_MAX_AGE,
				path: "/",
			});
		}

		return NextResponse.redirect(data.authorization_url);
	} catch (error) {
		console.error("Error initiating login:", error);
		return NextResponse.redirect(
			new URL("/?auth_error=login_failed", baseUrl),
		);
	}
}
