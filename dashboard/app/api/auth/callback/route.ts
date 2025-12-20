import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";
import { RELAY_API_URL } from "@/lib/config";

const STATE_COOKIE = "relay_oauth_state";
const SESSION_COOKIE = "relay_session";
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

	if (!code) {
		return NextResponse.redirect(new URL("/?auth_error=no_code", baseUrl));
	}

	try {
		const res = await fetch(`${RELAY_API_URL}/auth/exchange`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ code }),
		});

		if (!res.ok) {
			const errorData = await res.json().catch(() => ({}));
			console.error("Failed to exchange code:", res.status, errorData);
			return NextResponse.redirect(
				new URL("/?auth_error=exchange_failed", baseUrl),
			);
		}

		const data: ExchangeResponse = await res.json();

		cookieStore.set(SESSION_COOKIE, data.session_id, {
			httpOnly: true,
			secure: process.env.NODE_ENV === "production",
			sameSite: "lax",
			maxAge: SESSION_MAX_AGE,
			path: "/",
		});

		return NextResponse.redirect(new URL("/dashboard", baseUrl));
	} catch (error) {
		console.error("Error exchanging code:", error);
		return NextResponse.redirect(
			new URL("/?auth_error=callback_failed", baseUrl),
		);
	}
}
